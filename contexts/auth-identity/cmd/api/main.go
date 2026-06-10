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

	postgresURI := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/auth_db?sslmode=disable")
	redisURI := getEnv("REDIS_URL", "localhost:6379")
	serverPort := getEnv("SERVER_PORT", ":8080")

	db, err := postgres.NewDB(postgresURI)
	if err != nil {
		log.Fatalf("❌ Error crítico en Postgres: %v", err)
	}
	defer db.Pool.Close()
	log.Println("✅ Conexión a Postgres establecida con éxito (Pool configurado).")

	rClient := redisClient.NewClient(&redisClient.Options{
		Addr: redisURI,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Error crítico en Redis: %v", err)
	}
	log.Println("✅ Conexión a Redis en memoria establecida con éxito.")

	userRepo := postgres.NewPostgresUserRepository(db)
	tokenRepo := redis.NewRedisTokenRepository(rClient)

	uuidGenerator := func() string {
		bytes := make([]byte, 16)
		_, _ = rand.Read(bytes)
		return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
	}

	jwtSecret := getEnv("JWT_SECRET", "dev_secret_local")
	tokenService := &jwtTokenService{secret: []byte(jwtSecret)}
	eventPublisher := &stubEventPublisher{}

	registerUC := application.NewRegisterUserUseCase(userRepo, eventPublisher, uuidGenerator)
	loginPassUC := application.NewLoginPasswordUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)
	verifyEmailUC := application.NewVerifyEmailUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)

	googleClientID := getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret := getEnv("GOOGLE_CLIENT_SECRET", "")
	googleRedirectURI := getEnv("GOOGLE_REDIRECT_URI", "http://localhost:5500/loginregister.html")
	metaAppID := getEnv("META_APP_ID", "")

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

	authHandler := httpapi.NewAuthHandler(registerUC, loginPassUC, verifyEmailUC, loginGoogleUC, loginMetaUC, ssoConfig)
	router := httpapi.NewRouter(authHandler)

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

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// --- TOKEN SERVICE ---
// FIX: GenerateAccessToken ahora inyecta roles Y permisos en el JWT.
// El claim "roles" es un array de strings (ej: ["PROPIETARIO"]).
// El claim "permissions" es un array de strings (ej: ["property:create", "property:read"]).
// Los otros módulos (api-gateway, catalog, maintenance) deben leer estos claims
// para tomar decisiones de autorización sin consultar auth_db en cada request.

type jwtTokenService struct {
	secret []byte
}

func (s *jwtTokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	// Derivamos los permisos a partir de los roles para incluirlos en el JWT.
	// Esto evita que cada microservicio tenga que consultar la DB para saber qué puede hacer el usuario.
	permissions := derivePermissions(roles)

	claims := jwtlib.MapClaims{
		"sub":         userID,
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
		"roles":       roles,       // ← FIX: roles del usuario (ej: ["PROPIETARIO"])
		"permissions": permissions, // ← NUEVO: permisos derivados (ej: ["property:create"])
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

// derivePermissions mapea cada rol a su conjunto de permisos según la matriz RBAC del documento.
// Esta es la única fuente de verdad de permisos en el sistema.
// Cuando se agreguen tablas permissions/role_permissions a auth_db, esta función
// se reemplaza por una consulta a la DB — la firma del método no cambia.
func derivePermissions(roles []string) []string {
	permMap := map[string]bool{}

	rolePermissions := map[string][]string{
		"INTERESADO": {
			"property:read",
			"postulation:create",
		},
		"INQUILINO": {
			"property:read",
			"contract:read",
			"maintenance:create", // puede abrir ticket
			"maintenance:read",   // puede ver sus tickets
			"invoice:read",       // puede ver sus facturas/expensas
			"invoice:upload",     // puede subir comprobantes
			"postulation:create", // puede postularse
			"message:create",     // puede enviar mensajes
			"message:read",       // puede leer mensajes
		},
		"PROPIETARIO": {
			"property:read",
			"property:create", // puede publicar propiedades
			"property:update", // puede editar sus propiedades
			"contract:read",
			"contract:update",    // puede aprobar/rechazar contratos
			"maintenance:read",   // puede ver tickets de sus propiedades
			"maintenance:update", // puede aprobar presupuestos
			"invoice:read",
			"invoice:create",
			"message:create",
			"message:read",
			"ledger:read",   // puede ver liquidaciones
			"ledger:create", // puede cargar pagos
		},
		"AGENTE": {
			"property:read",
			"property:create",
			"property:update",
			"property:delete",
			"postulation:read",
			"postulation:update",
			"contract:read",
			"contract:create",
			"contract:update",
			"maintenance:read",
			"maintenance:update",
			"invoice:read",
			"message:create",
			"message:read",
		},
		"PROVEEDOR": {
			"maintenance:read",   // puede ver órdenes asignadas
			"maintenance:update", // puede actualizar estado y subir presupuesto
		},
		"ADMIN_INMO": {
			"property:read",
			"property:create",
			"property:update",
			"property:delete",
			"postulation:read",
			"postulation:update",
			"contract:read",
			"contract:create",
			"contract:update",
			"contract:delete",
			"maintenance:read",
			"maintenance:create",
			"maintenance:update",
			"maintenance:delete",
			"invoice:read",
			"invoice:create",
			"invoice:update",
			"invoice:delete",
			"ledger:read",
			"ledger:create",
			"ledger:update",
			"message:create",
			"message:read",
		},
		"ROOT": {
			"tenant:create",
			"tenant:read",
			"tenant:update",
			"tenant:delete",
			"metrics:read",
		},
	}

	for _, role := range roles {
		if perms, ok := rolePermissions[role]; ok {
			for _, p := range perms {
				permMap[p] = true
			}
		}
	}

	// Convertir el map a slice para el claim del JWT
	result := make([]string, 0, len(permMap))
	for perm := range permMap {
		result = append(result, perm)
	}
	return result
}

type stubEventPublisher struct{}

func (s *stubEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	log.Printf("[NATS BUS OUT] Evento '%s' para usuario: %s\n", event.Name, event.UserID)
	return nil
}

type stubIdentityService struct{}

func (s *stubIdentityService) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	log.Printf("📥 [STUB GOOGLE] code recibido: %s\n", code)
	return &ports.SSOResult{
		ProviderUserID: "google-uid-mock-123456",
		Email:          "diego.maradona@example.com",
	}, nil
}

func (s *stubIdentityService) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	return nil, fmt.Errorf("no implementado en stub")
}
