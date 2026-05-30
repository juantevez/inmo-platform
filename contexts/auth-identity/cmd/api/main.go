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
	"inmo.platform/contexts/auth-identity/internal/adapters/postgres"
	"inmo.platform/contexts/auth-identity/internal/adapters/redis"
	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/ports"

	// Driver oficial de Redis y Postgres estándar
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

	tokenService := &stubTokenService{}
	eventPublisher := &stubEventPublisher{}

	// 6. Inyección de Dependencias: Instanciar los Casos de Uso (Application Layer)
	registerUC := application.NewRegisterUserUseCase(userRepo, eventPublisher, uuidGenerator)
	loginPassUC := application.NewLoginPasswordUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)
	verifyEmailUC := application.NewVerifyEmailUseCase(userRepo, tokenRepo, tokenService, eventPublisher, uuidGenerator)

	// Nota: Si pasás las credenciales de Google Developer Console por Env, instanciás el adapter real acá
	loginGoogleUC := application.NewLoginSSOGoogleUseCase(userRepo, tokenRepo, nil, tokenService, eventPublisher, uuidGenerator)

	// 7. Conectar la capa HTTP entubando los Casos de Uso al enrutador nativo
	authHandler := httpapi.NewAuthHandler(registerUC, loginPassUC, verifyEmailUC, loginGoogleUC)
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

// --- IMPLEMENTACIONES TEMPORALES (STUBS) PARA PERMITIR COMPILACIÓN INMEDIATA ---

type stubTokenService struct{}

func (s *stubTokenService) GenerateAccessToken(userID string) (string, error) {
	// Simula la firma de un JWT corto (15 min). En producción usarás "github.com/golang-jwt/jwt/v5"
	return "jwt.mock.access_token.for_user_" + userID, nil
}

func (s *stubTokenService) GenerateRefreshToken() (string, error) {
	// Genera un token aleatorio seguro de 32 bytes para las sesiones de Redis
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

type stubEventPublisher struct{}

func (s *stubEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	// Simula la salida hacia NATS JetStream imprimiendo en la consola local
	log.Printf("[NATS BUS OUT] Evento publicado en el subject '%s' para el usuario: %s\n", event.Name, event.UserID)
	return nil
}
