package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	// Importamos nuestros paquetes internos del Bounded Context
	"inmo.platform/contexts/auth-identity/internal/adapters/httpapi"
	"inmo.platform/contexts/auth-identity/internal/adapters/oauth"
	"inmo.platform/contexts/auth-identity/internal/adapters/postgres"
	"inmo.platform/contexts/auth-identity/internal/adapters/redis"
	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/ports"

	jwtlib "github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	redisClient "github.com/redis/go-redis/v9"
)

func main() {
	log.Println("🚀 Iniciando el Bounded Context: auth-identity...")

	// 1. Cargar configuraciones básicas desde Variables de Entorno (o usar fallbacks locales)
	postgresURI := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/auth_db?sslmode=disable")
	redisURI := getEnv("REDIS_URL", "localhost:6379")
	serverPort := getEnv("SERVER_PORT", ":8080")

	// 2. Inicializar Infraestructura: Pool de conexiones a Postgres
	db, err := postgres.NewDB(postgresURI)
	if err != nil {
		log.Fatalf("❌ Error crítico en Postgres: %v", err)
	}
	defer db.Pool.Close()
	log.Println("✅ Conexión a Postgres establecida con éxito (Pool configurado).")

	// 3. Inicializar Infraestructura: Cliente de Redis
	rClient := redisClient.NewClient(&redisClient.Options{
		Addr: redisURI,
	})
	// Validamos conectividad con un Ping rápido bajo timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Error crítico en Redis: %v", err)
	}
	log.Println("✅ Conexión a Redis en memoria establecida con éxito.")

	// 4. Instanciar los Adaptadores de Persistencia (Repositories)
	userRepo := postgres.NewPostgresUserRepository(db)
	tokenRepo := redis.NewRedisTokenRepository(rClient)

	// 5. Instanciar Proveedores de Servicios Auxiliares (Simulados para compilación directa)
	uuidGenerator := func() string {
		bytes := make([]byte, 16)
		_, _ = rand.Read(bytes)
		return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
	}

	jwtSecret := getEnv("JWT_SECRET", "dev_secret_local")
	tokenService := &jwtTokenService{secret: []byte(jwtSecret)}
	eventPublisher := &stubEventPublisher{}

	// 6. Inyección de Dependencias: Instanciar los Casos de Uso (Application Layer)
	registerUC := application.NewRegisterUserUseCase(userRepo, eventPublisher, uuidGenerator)
	loginPassUC := application.NewLoginPasswordUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)
	verifyEmailUC := application.NewVerifyEmailUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)

	// Credenciales OAuth desde variables de entorno
	googleClientID    := getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret := getEnv("GOOGLE_CLIENT_SECRET", "")
	googleRedirectURI  := getEnv("GOOGLE_REDIRECT_URI", "http://localhost:5500/loginregister.html")
	metaAppID         := getEnv("META_APP_ID", "")

	// Seleccionar adaptador de Google: real si hay credenciales, stub si no
	var googleIdentity ports.IdentityService
	if googleClientID != "" && googleClientSecret != "" {
		googleIdentity = oauth.NewGoogleAdapter(googleClientID, googleClientSecret, googleRedirectURI)
	} else {
		googleIdentity = &stubIdentityService{}
	}

	metaAdapter := oauth.NewMetaAdapter()

	loginGoogleUC := application.NewLoginSSOGoogleUseCase(userRepo, tokenRepo, googleIdentity, tokenService, eventPublisher, uuidGenerator)
	loginMetaUC := application.NewLoginSSOMetaUseCase(userRepo, tokenRepo, metaAdapter, tokenService, eventPublisher, uuidGenerator)

	ssoConfig := httpapi.SSOPublicConfig{
		GoogleClientID:    googleClientID,
		GoogleRedirectURI: googleRedirectURI,
		MetaAppID:         metaAppID,
	}

	// 7. Conectar la capa HTTP entubando los Casos de Uso al enrutador nativo
	authHandler := httpapi.NewAuthHandler(registerUC, loginPassUC, verifyEmailUC, loginGoogleUC, loginMetaUC, ssoConfig)
	router := httpapi.NewRouter(authHandler)

	// 8. Levantar el servidor HTTP escuchando en el puerto configurado
	server := &http.Server{
		Addr:         serverPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("🌍 Servidor HTTP de Autenticación corriendo en http://localhost%s", serverPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Fallo catastrófico en el servidor HTTP: %v", err)
	}
}

// Helper simple para leer variables de entorno con fallback predeterminado
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// --- TOKEN SERVICE ---

type jwtTokenService struct {
	secret []byte
}

func (s *jwtTokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	claims := jwtlib.MapClaims{
		"sub":  userID,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *jwtTokenService) GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

type stubEventPublisher struct{}

func (s *stubEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	log.Printf("[NATS BUS OUT] Evento publicado en el subject '%s' para el usuario: %s\n", event.Name, event.UserID)
	return nil
}

type stubIdentityService struct{}

func (s *stubIdentityService) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	log.Println("==================================================")
	log.Printf("📥 ¡ENTRANDO AL STUB DE GOOGLE! El 'code' recibido es: %s\n", code)
	log.Println("==================================================")

	return &ports.SSOResult{
		ProviderUserID: "google-uid-mock-123456",
		Email:          "diego.maradona@example.com",
	}, nil
}

func (s *stubIdentityService) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	return nil, fmt.Errorf("no implementado en stub")
}
