package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newVerifyEmailUseCase(userRepo ports.UserRepository, tokenRepo ports.TokenRepository, tokenSvc application.TokenService, publisher ports.EventPublisher, uuidGen application.UUIDGenerator) *application.VerifyEmailUseCase {
	return application.NewVerifyEmailUseCase(userRepo, tokenRepo, tokenSvc, publisher, uuidGen)
}

// expiredToken construye un VerificationToken de EMAIL_VERIFICATION ya vencido.
func expiredToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	return domain.ReconstructVerificationToken(value, domain.TypeEmailVerification, userID, time.Now().Add(-1*time.Hour), nil)
}

// usedToken construye un VerificationToken de EMAIL_VERIFICATION ya consumido.
func usedToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	usedAt := time.Now().Add(-1 * time.Hour)
	return domain.ReconstructVerificationToken(value, domain.TypeEmailVerification, userID, time.Now().Add(23*time.Hour), &usedAt)
}

// validToken construye un VerificationToken de EMAIL_VERIFICATION vigente y sin usar.
func validToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	return domain.ReconstructVerificationToken(value, domain.TypeEmailVerification, userID, time.Now().Add(24*time.Hour), nil)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestVerifyEmailExecute_ErrorBuscandoToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return nil, boom
		},
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestVerifyEmailExecute_TokenInexistente_RetornaErrTokenNotFound(t *testing.T) {
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			if tType != domain.TypeEmailVerification {
				t.Errorf("FindVerificationToken tType: got %q, want %q", tType, domain.TypeEmailVerification)
			}
			return nil, nil
		},
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "no-existe"})

	if !errors.Is(err, application.ErrTokenNotFound) {
		t.Fatalf("Execute: got %v, want ErrTokenNotFound", err)
	}
}

func TestVerifyEmailExecute_TokenExpirado_RetornaMensajeDeExpiracion(t *testing.T) {
	tok := expiredToken(t, "tok-1", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if err == nil || !strings.Contains(err.Error(), "expirado") {
		t.Fatalf("Execute: got %v, want mensaje de expiración", err)
	}
}

func TestVerifyEmailExecute_TokenYaUsado_RetornaErrTokenAlreadyUsed(t *testing.T) {
	tok := usedToken(t, "tok-1", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	// A diferencia de la expiración, el reuso se propaga tal cual desde el dominio (sin mensaje custom).
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Fatalf("Execute: got %v, want %v", err, domain.ErrTokenAlreadyUsed)
	}
}

func TestVerifyEmailExecute_ErrorBuscandoUsuario_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	boom := errors.New("timeout de base de datos")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return nil, boom },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestVerifyEmailExecute_UsuarioYaNoExiste_RetornaError(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return nil, nil },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if err == nil || !strings.Contains(err.Error(), "ya no existe") {
		t.Fatalf("Execute: got %v, want error de usuario inexistente", err)
	}
}

func TestVerifyEmailExecute_UsuarioSuspendido_RetornaErrUserSuspended(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	suspended := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return suspended, nil },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(err, domain.ErrUserSuspended) {
		t.Fatalf("Execute: got %v, want %v", err, domain.ErrUserSuspended)
	}
}

func TestVerifyEmailExecute_UsuarioYaActivo_RetornaErrUserAlreadyActive(t *testing.T) {
	// Caso de token válido pero apuntando a un usuario que ya fue activado (ej: doble clic en el link).
	tok := validToken(t, "tok-1", "user-1")
	active, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return active, nil },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, domain.ErrUserAlreadyActive) {
		t.Fatalf("Execute: got %v, want %v", execErr, domain.ErrUserAlreadyActive)
	}
}

func TestVerifyEmailExecute_ErrorActualizandoUsuario_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	boom := errors.New("fallo de escritura")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn:   func(ctx context.Context, user *domain.User) error { return boom },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestVerifyEmailExecute_ErrorActualizandoToken_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	boom := errors.New("fallo de escritura en verification_tokens")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn:                  func(ctx context.Context, user *domain.User) error { return nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return boom },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestVerifyEmailExecute_ErrorGenerandoAccessToken_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	boom := errors.New("clave de firma inválida")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn:                  func(ctx context.Context, user *domain.User) error { return nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return nil },
	}
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) { return "", boom },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestVerifyEmailExecute_ErrorGenerandoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	boom := errors.New("fallo generando refresh token")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn:                  func(ctx context.Context, user *domain.User) error { return nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return nil },
	}
	tokenSvc := &fakeTokenService{
		generateRefreshFn: func() (string, error) { return "", boom },
	}
	uc := newVerifyEmailUseCase(userRepo, &fakeTokenRepo{}, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestVerifyEmailExecute_ErrorPersistiendoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	boom := errors.New("redis no disponible")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn:                  func(ctx context.Context, user *domain.User) error { return nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return nil },
	}
	tokenRepo := &fakeTokenRepo{
		setRefreshFn: func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error { return boom },
	}
	uc := newVerifyEmailUseCase(userRepo, tokenRepo, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestVerifyEmailExecute_HappyPath_ActivaUsuarioYEmiteTokens(t *testing.T) {
	tok := validToken(t, "tok-1", "user-1")
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}

	var updatedUser *domain.User
	var updatedToken *domain.VerificationToken
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			if tokenValue != "tok-1" {
				t.Errorf("FindVerificationToken tokenValue: got %q, want %q", tokenValue, "tok-1")
			}
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return pending, nil },
		updateFn: func(ctx context.Context, user *domain.User) error {
			updatedUser = user
			return nil
		},
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error {
			updatedToken = token
			return nil
		},
	}
	tokenRepo := &fakeTokenRepo{}
	publisher := &fakeEventPublisher{}
	uc := newVerifyEmailUseCase(userRepo, tokenRepo, &fakeTokenService{}, publisher, fixedUUIDGen("evt-1"))

	resp, execErr := uc.Execute(context.Background(), application.VerifyEmailCommand{TokenValue: "tok-1"})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if resp.AccessToken != "access-token" || resp.RefreshToken != "refresh-token" {
		t.Errorf("Response: got %+v, want tokens por defecto del fake", resp)
	}

	// El usuario pasado a Update() debe reflejar la activación
	if updatedUser == nil {
		t.Fatal("Update: no fue invocado")
	}
	if updatedUser.Status() != domain.StatusActive {
		t.Errorf("Update user.Status(): got %q, want %q", updatedUser.Status(), domain.StatusActive)
	}

	// El token pasado a UpdateVerificationToken() debe quedar marcado como usado
	if updatedToken == nil {
		t.Fatal("UpdateVerificationToken: no fue invocado")
	}
	if updatedToken.UsedAt() == nil {
		t.Error("UpdateVerificationToken token.UsedAt(): no debería ser nil tras Use()")
	}

	// El refresh token nuevo se persiste con TTL de 7 días bajo el usuario activado
	if !tokenRepo.setRefreshCalled || tokenRepo.setRefreshUserID != "user-1" || tokenRepo.setRefreshTTL != 7*24*time.Hour {
		t.Errorf("SetRefreshToken: got called=%v userID=%q ttl=%v",
			tokenRepo.setRefreshCalled, tokenRepo.setRefreshUserID, tokenRepo.setRefreshTTL)
	}

	// Se publica el evento de verificación exitosa con el email del usuario
	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.Name != "auth.email.verified" {
		t.Errorf("Name: got %q, want %q", evt.Name, "auth.email.verified")
	}
	if evt.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", evt.UserID, "user-1")
	}
	if evt.Payload["email"] != "user@test.com" {
		t.Errorf("Payload[email]: got %v, want %q", evt.Payload["email"], "user@test.com")
	}
}
