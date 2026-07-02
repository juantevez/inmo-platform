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

func newVerifyPhoneOTPUseCase(userRepo ports.UserRepository) *application.VerifyPhoneOTPUseCase {
	return application.NewVerifyPhoneOTPUseCase(userRepo)
}

// validOTPToken construye un VerificationToken de PHONE_OTP vigente y sin usar.
func validOTPToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	return domain.ReconstructVerificationToken(value, domain.TypePhoneOTP, userID, time.Now().Add(10*time.Minute), nil)
}

// expiredOTPToken construye un VerificationToken de PHONE_OTP ya vencido.
func expiredOTPToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	return domain.ReconstructVerificationToken(value, domain.TypePhoneOTP, userID, time.Now().Add(-1*time.Minute), nil)
}

// usedOTPToken construye un VerificationToken de PHONE_OTP ya consumido.
func usedOTPToken(t *testing.T, value, userID string) *domain.VerificationToken {
	t.Helper()
	usedAt := time.Now().Add(-1 * time.Minute)
	return domain.ReconstructVerificationToken(value, domain.TypePhoneOTP, userID, time.Now().Add(9*time.Minute), &usedAt)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestVerifyPhoneOTPExecute_OTPVacio_RetornaErrOTPNotFound(t *testing.T) {
	uc := newVerifyPhoneOTPUseCase(&fakeUserRepo{})

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: ""})

	if !errors.Is(err, application.ErrOTPNotFound) {
		t.Fatalf("Execute: got %v, want ErrOTPNotFound", err)
	}
}

func TestVerifyPhoneOTPExecute_ErrorBuscandoToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			if tType != domain.TypePhoneOTP {
				t.Errorf("FindVerificationToken tType: got %q, want %q", tType, domain.TypePhoneOTP)
			}
			return nil, boom
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestVerifyPhoneOTPExecute_TokenInexistente_RetornaErrOTPNotFound(t *testing.T) {
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return nil, nil
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if !errors.Is(err, application.ErrOTPNotFound) {
		t.Fatalf("Execute: got %v, want ErrOTPNotFound", err)
	}
}

func TestVerifyPhoneOTPExecute_TokenPerteneceAOtroUsuario_RetornaErrOTPNotFound(t *testing.T) {
	// Seguridad estricta: un OTP válido pero emitido para otro UserID debe rechazarse
	// con el mismo mensaje genérico que "no existe", para no filtrar información.
	tok := validOTPToken(t, "123456", "otro-usuario")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if !errors.Is(err, application.ErrOTPNotFound) {
		t.Fatalf("Execute: got %v, want ErrOTPNotFound", err)
	}
}

func TestVerifyPhoneOTPExecute_TokenExpirado_RetornaMensajeDeExpiracion(t *testing.T) {
	tok := expiredOTPToken(t, "123456", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if err == nil || !strings.Contains(err.Error(), "expirado") {
		t.Fatalf("Execute: got %v, want mensaje de expiración", err)
	}
}

func TestVerifyPhoneOTPExecute_TokenYaUsado_RetornaErrTokenAlreadyUsed(t *testing.T) {
	tok := usedOTPToken(t, "123456", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	// A diferencia de la expiración, el reuso se propaga tal cual desde el dominio (sin mensaje custom).
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Fatalf("Execute: got %v, want %v", err, domain.ErrTokenAlreadyUsed)
	}
}

func TestVerifyPhoneOTPExecute_ErrorBuscandoUsuario_RetornaUsuarioNoEncontrado(t *testing.T) {
	tok := validOTPToken(t, "123456", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) {
			return nil, errors.New("timeout de base de datos")
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if err == nil || err.Error() != "usuario no encontrado" {
		t.Fatalf("Execute: got %v, want %q", err, "usuario no encontrado")
	}
}

func TestVerifyPhoneOTPExecute_UsuarioNoExiste_RetornaUsuarioNoEncontrado(t *testing.T) {
	tok := validOTPToken(t, "123456", "user-1")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) { return nil, nil },
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if err == nil || err.Error() != "usuario no encontrado" {
		t.Fatalf("Execute: got %v, want %q", err, "usuario no encontrado")
	}
}

func TestVerifyPhoneOTPExecute_ErrorActualizandoToken_RetornaErrorEnvuelto(t *testing.T) {
	tok := validOTPToken(t, "123456", "user-1")
	user := userWithPhone(t, "user-1", "+541112345678")
	boom := errors.New("fallo de escritura en verification_tokens")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return boom },
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestVerifyPhoneOTPExecute_ErrorActualizandoUsuario_RetornaErrorEnvuelto(t *testing.T) {
	tok := validOTPToken(t, "123456", "user-1")
	user := userWithPhone(t, "user-1", "+541112345678")
	boom := errors.New("fallo guardando phone_verified_at")
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			return tok, nil
		},
		findByIDFn:                func(ctx context.Context, id string) (*domain.User, error) { return user, nil },
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return nil },
		updateFn:                  func(ctx context.Context, user *domain.User) error { return boom },
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestVerifyPhoneOTPExecute_HappyPath_QuemaOTPYActualizaUsuario(t *testing.T) {
	tok := validOTPToken(t, "123456", "user-1")
	user := userWithPhone(t, "user-1", "+541112345678")

	var updatedToken *domain.VerificationToken
	var updatedUser *domain.User
	userRepo := &fakeUserRepo{
		findVerificationTokenFn: func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
			if tokenValue != "123456" {
				t.Errorf("FindVerificationToken tokenValue: got %q, want %q", tokenValue, "123456")
			}
			return tok, nil
		},
		findByIDFn: func(ctx context.Context, id string) (*domain.User, error) {
			if id != "user-1" {
				t.Errorf("FindByID: got %q, want %q", id, "user-1")
			}
			return user, nil
		},
		updateVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error {
			updatedToken = token
			return nil
		},
		updateFn: func(ctx context.Context, user *domain.User) error {
			updatedUser = user
			return nil
		},
	}
	uc := newVerifyPhoneOTPUseCase(userRepo)

	err := uc.Execute(context.Background(), application.VerifyPhoneOTPCommand{UserID: "user-1", OTPValue: "123456"})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	// El token debe quedar marcado como usado antes de persistirse
	if updatedToken == nil {
		t.Fatal("UpdateVerificationToken: no fue invocado")
	}
	if updatedToken.UsedAt() == nil {
		t.Error("UpdateVerificationToken token.UsedAt(): no debería ser nil tras Use()")
	}

	// El mismo usuario recuperado se pasa a Update() para que el adapter marque phone_verified_at
	if updatedUser == nil {
		t.Fatal("Update: no fue invocado")
	}
	if updatedUser.ID() != "user-1" {
		t.Errorf("Update user.ID(): got %q, want %q", updatedUser.ID(), "user-1")
	}
}
