package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	redisClient "github.com/redis/go-redis/v9"
)

func main() {
	log.Println("🚀 Iniciando el Bounded Context: auth-identity...")

	postgresURI := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/auth_db?sslmode=disable")
	redisURI := getEnv("REDIS_URL", "localhost:6379")
	serverPort := getEnv("SERVER_PORT", ":8080")
	natsURL := getEnv("NATS_URL", nats.DefaultURL)

	// 1. Postgres
	db, err := postgres.NewDB(postgresURI)
	if err != nil {
		log.Fatalf("❌ Error crítico en Postgres: %v", err)
	}
	defer db.Pool.Close()
	log.Println("✅ Conexión a Postgres establecida con éxito.")

	// 2. Redis
	rClient := redisClient.NewClient(&redisClient.Options{Addr: redisURI})
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pingCancel()
	if err := rClient.Ping(pingCtx).Err(); err != nil {
		log.Fatalf("❌ Error crítico en Redis: %v", err)
	}
	log.Println("✅ Conexión a Redis establecida con éxito.")

	// 3. NATS JetStream — reemplaza el stubEventPublisher
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(5),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("❌ Error crítico en NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("❌ Error crítico al inicializar JetStream: %v", err)
	}

	// Crear stream "auth" — idempotente, si ya existe lo actualiza
	initCtx, initCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = js.CreateOrUpdateStream(initCtx, jetstream.StreamConfig{
		Name:      "auth",
		Subjects:  []string{"auth.user.*"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	initCancel()
	if err != nil {
		log.Printf("⚠️  No se pudo crear/validar el stream 'auth': %v", err)
	} else {
		log.Println("✅ NATS JetStream listo. Stream 'auth' validado.")
	}

	// 4. Repositorios y servicios
	userRepo := postgres.NewPostgresUserRepository(db)
	tokenRepo := redis.NewRedisTokenRepository(rClient)

	uuidGenerator := func() string {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	}

	jwtSecret := getEnv("JWT_SECRET", "dev_secret_local")
	tokenService := &jwtTokenService{secret: []byte(jwtSecret)}

	// Publisher real contra NATS — ya no es un stub
	eventPublisher := newNatsEventPublisher(js)

	// 5. Casos de uso
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

	// 6. HTTP
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

// =========================================================================
// NATS Event Publisher — implementa ports.EventPublisher
// =========================================================================

type natsEventPublisher struct {
	js jetstream.JetStream
}

func newNatsEventPublisher(js jetstream.JetStream) *natsEventPublisher {
	return &natsEventPublisher{js: js}
}

// wireAuthEvent es el DTO de serialización para el wire format de NATS.
// El subject usado es event.Name — ej: "auth.user.created".
type wireAuthEvent struct {
	EventID   string                 `json:"event_id"`
	EventName string                 `json:"event_name"`
	UserID    string                 `json:"user_id"`
	Timestamp string                 `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

func (p *natsEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	wire := wireAuthEvent{
		EventID:   event.EventID,
		EventName: event.Name,
		UserID:    event.UserID,
		Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
		Payload:   event.Payload,
	}

	payload, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("error al serializar AuthEvent '%s': %w", event.Name, err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	ack, err := p.js.Publish(pubCtx, event.Name, payload)
	if err != nil {
		return fmt.Errorf("error al publicar evento '%s' en NATS: %w", event.Name, err)
	}

	log.Printf("[NATS OUT] '%s' publicado | Stream: %s | Seq: %d | UserID: %s",
		event.Name, ack.Stream, ack.Sequence, event.UserID)
	return nil
}

// =========================================================================
// JWT Token Service
// =========================================================================

type jwtTokenService struct {
	secret []byte
}

func (s *jwtTokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	permissions := derivePermissions(roles)
	claims := jwtlib.MapClaims{
		"sub":         userID,
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
		"roles":       roles,
		"permissions": permissions,
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *jwtTokenService) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func derivePermissions(roles []string) []string {
	permMap := map[string]bool{}
	rolePermissions := map[string][]string{
		"INTERESADO":  {"property:read", "postulation:create"},
		"INQUILINO":   {"property:read", "contract:read", "maintenance:create", "maintenance:read", "invoice:read", "invoice:upload", "postulation:create", "message:create", "message:read"},
		"PROPIETARIO": {"property:read", "property:create", "property:update", "contract:read", "contract:update", "maintenance:read", "maintenance:update", "invoice:read", "invoice:create", "message:create", "message:read", "ledger:read", "ledger:create"},
		"AGENTE":      {"property:read", "property:create", "property:update", "property:delete", "postulation:read", "postulation:update", "contract:read", "contract:create", "contract:update", "maintenance:read", "maintenance:update", "invoice:read", "message:create", "message:read"},
		"PROVEEDOR":   {"maintenance:read", "maintenance:update"},
		"ADMIN_INMO":  {"property:read", "property:create", "property:update", "property:delete", "postulation:read", "postulation:update", "contract:read", "contract:create", "contract:update", "contract:delete", "maintenance:read", "maintenance:create", "maintenance:update", "maintenance:delete", "invoice:read", "invoice:create", "invoice:update", "invoice:delete", "ledger:read", "ledger:create", "ledger:update", "message:create", "message:read"},
		"ROOT":        {"tenant:create", "tenant:read", "tenant:update", "tenant:delete", "metrics:read"},
	}
	for _, role := range roles {
		for _, p := range rolePermissions[role] {
			permMap[p] = true
		}
	}
	result := make([]string, 0, len(permMap))
	for perm := range permMap {
		result = append(result, perm)
	}
	return result
}

// =========================================================================
// Stubs para desarrollo
// =========================================================================

type stubIdentityService struct{}

func (s *stubIdentityService) VerifyGoogleCode(_ context.Context, code string) (*ports.SSOResult, error) {
	log.Printf("📥 [STUB GOOGLE] code recibido: %s\n", code)
	return &ports.SSOResult{
		ProviderUserID: "google-uid-mock-123456",
		Email:          "diego.maradona@example.com",
	}, nil
}

func (s *stubIdentityService) VerifyMetaToken(_ context.Context, _ string) (*ports.SSOResult, error) {
	return nil, fmt.Errorf("no implementado en stub")
}
