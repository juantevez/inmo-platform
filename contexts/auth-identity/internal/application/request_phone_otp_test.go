package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newRequestPhoneOTPUseCase(userRepo ports.UserRepository, tokenRepo ports.TokenRepository, publisher ports.EventPublisher, uuidGen application.UUIDGenerator) *application.RequestPhoneOTPUseCase {
	return application.NewRequestPhoneOTPUseCase(userRepo, tokenRepo, publisher, uuidGen)
}

// userWithPhone arma un usuario ACTIVE con un teléfono cargado, vía ReconstructUser
// (igual que lo haría la capa de infraestructura al hidratar desde Postgres).
func userWithPhone(t *testing.T, id, phone string) *domain.User {
	t.Helper()
	return domain.ReconstructUser(id, "user@test.com", string(domain.StatusActive), phone, nil, time.Now())
}

func isNumericOTP(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestRequestPhoneOTP_CanalInvalido_RetornaErrInvalidChannel(t *testing.T) {
	uc := newRequestPhoneOTPUseCase(&fakeUserRepo{}, &fakeTokenRepo{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "EMAIL"})

	if !errors.Is(err, application.ErrInvalidChannel) {
		t.Fatalf("Execute: got %v, want ErrInvalidChannel", err)
	}
}

func TestRequestPhoneOTP_CanalVacio_RetornaErrInvalidChannel(t *testing.T) {
	uc := newRequestPhoneOTPUseCase(&fakeUserRepo{}, &fakeTokenRepo{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: ""})

	if !errors.Is(err, application.ErrInvalidChannel) {
		t.Fatalf("Execute: got %v, want ErrInvalidChannel", err)
	}
}

func TestRequestPhoneOTP_ErrorBuscandoUsuario_RetornaUsuarioNoEncontrado(t *testing.T) {
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) {
			return nil, errors.New("timeout de base de datos")
		},
	}
	uc := newRequestPhoneOTPUseCase(userRepo, &fakeTokenRepo{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if err == nil || err.Error() != "usuario no encontrado" {
		t.Fatalf("Execute: got %v, want %q", err, "usuario no encontrado")
	}
}

func TestRequestPhoneOTP_UsuarioNoExiste_RetornaUsuarioNoEncontrado(t *testing.T) {
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return nil, nil },
	}
	uc := newRequestPhoneOTPUseCase(userRepo, &fakeTokenRepo{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "WHATSAPP"})

	if err == nil || err.Error() != "usuario no encontrado" {
		t.Fatalf("Execute: got %v, want %q", err, "usuario no encontrado")
	}
}

func TestRequestPhoneOTP_UsuarioSinTelefono_RetornaError(t *testing.T) {
	user := userWithPhone(t, "user-1", "")
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
	}
	uc := newRequestPhoneOTPUseCase(userRepo, &fakeTokenRepo{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if err == nil || err.Error() != "el usuario no posee un número de teléfono registrado en su cuenta" {
		t.Fatalf("Execute: got %v, want error de teléfono no registrado", err)
	}
}

func TestRequestPhoneOTP_ErrorEnRateLimit_RetornaErrorEnvuelto(t *testing.T) {
	user := userWithPhone(t, "user-1", "+541112345678")
	boom := errors.New("redis caído")
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
	}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) { return 0, boom },
	}
	uc := newRequestPhoneOTPUseCase(userRepo, tokenRepo, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestRequestPhoneOTP_RateLimitExcedido_RetornaErrOTPMaxAttemptsExceeded(t *testing.T) {
	user := userWithPhone(t, "user-1", "+541112345678")
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
	}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) { return 4, nil },
	}
	uc := newRequestPhoneOTPUseCase(userRepo, tokenRepo, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if !errors.Is(err, application.ErrOTPMaxAttemptsExceeded) {
		t.Fatalf("Execute: got %v, want ErrOTPMaxAttemptsExceeded", err)
	}
}

func TestRequestPhoneOTP_LimiteDeTresEsPermitido(t *testing.T) {
	// El corte es "> 3", así que el tercer intento (attempts == 3) todavía debe pasar.
	user := userWithPhone(t, "user-1", "+541112345678")
	var savedToken *domain.VerificationToken
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
		saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error {
			savedToken = token
			return nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) { return 3, nil },
	}
	uc := newRequestPhoneOTPUseCase(userRepo, tokenRepo, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if err != nil {
		t.Fatalf("Execute: error inesperado con attempts=3: %v", err)
	}
	if savedToken == nil {
		t.Fatal("SaveVerificationToken: no fue invocado")
	}
}

func TestRequestPhoneOTP_ErrorGuardandoToken_RetornaErrorEnvuelto(t *testing.T) {
	user := userWithPhone(t, "user-1", "+541112345678")
	boom := errors.New("fallo de escritura en verification_tokens")
	userRepo := &fakeUserRepo{
		findByIDFn:              func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
		saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return boom },
	}
	tokenRepo := &fakeTokenRepo{}
	uc := newRequestPhoneOTPUseCase(userRepo, tokenRepo, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "SMS"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestRequestPhoneOTP_HappyPath_GeneraOTPYEmiteEvento(t *testing.T) {
	user := userWithPhone(t, "user-1", "+541112345678")
	var savedToken *domain.VerificationToken
	userRepo := &fakeUserRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) {
			if id != "user-1" {
				t.Errorf("FindByID: got %q, want %q", id, "user-1")
			}
			return user, nil
		},
		saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error {
			savedToken = token
			return nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) {
			wantKey := "otp_limit:+541112345678"
			if key != wantKey {
				t.Errorf("IncrementLoginAttempts key: got %q, want %q", key, wantKey)
			}
			if window != time.Hour {
				t.Errorf("IncrementLoginAttempts window: got %v, want %v", window, time.Hour)
			}
			return 1, nil
		},
	}
	publisher := &fakeEventPublisher{}
	uc := newRequestPhoneOTPUseCase(userRepo, tokenRepo, publisher, fixedUUIDGen("evt-1"))

	err := uc.Execute(context.Background(), application.RequestPhoneOTPCommand{UserID: "user-1", Channel: "WHATSAPP"})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// El token generado debe ser un OTP numérico de 6 dígitos con TTL de tipo PHONE_OTP
	if savedToken == nil {
		t.Fatal("SaveVerificationToken: no fue invocado")
	}
	if !isNumericOTP(savedToken.Value(), 6) {
		t.Errorf("OTP generado: got %q, want 6 dígitos numéricos", savedToken.Value())
	}
	if savedToken.Type() != domain.TypePhoneOTP {
		t.Errorf("Token.Type(): got %q, want %q", savedToken.Type(), domain.TypePhoneOTP)
	}
	if savedToken.UserID() != "user-1" {
		t.Errorf("Token.UserID(): got %q, want %q", savedToken.UserID(), "user-1")
	}

	// El evento de auditoría lleva el teléfono, el OTP generado y el canal solicitado
	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.Name != "auth.phone_otp.requested" {
		t.Errorf("Name: got %q, want %q", evt.Name, "auth.phone_otp.requested")
	}
	if evt.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", evt.UserID, "user-1")
	}
	if evt.Payload["phone"] != "+541112345678" {
		t.Errorf("Payload[phone]: got %v, want %q", evt.Payload["phone"], "+541112345678")
	}
	if evt.Payload["otp"] != savedToken.Value() {
		t.Errorf("Payload[otp]: got %v, want %q", evt.Payload["otp"], savedToken.Value())
	}
	if evt.Payload["channel"] != "WHATSAPP" {
		t.Errorf("Payload[channel]: got %v, want %q", evt.Payload["channel"], "WHATSAPP")
	}
}
