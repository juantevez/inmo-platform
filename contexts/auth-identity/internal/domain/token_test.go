package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/domain"
)

// ─── helpers de construcción ──────────────────────────────────────────────────

// makeExpiredEmailToken construye un token de verificación de email cuyo
// expiresAt ya pasó, usando ReconstructVerificationToken para bypassear
// el constructor que siempre setea TTL en el futuro.
func makeExpiredEmailToken(tokenValue, userID string) *domain.VerificationToken {
	past := time.Now().Add(-25 * time.Hour) // más de 24h → expirado
	return domain.ReconstructVerificationToken(
		tokenValue,
		domain.TypeEmailVerification,
		userID,
		past,
		nil,
	)
}

// makeUsedEmailToken construye un token marcado como ya utilizado.
func makeUsedEmailToken(tokenValue, userID string) *domain.VerificationToken {
	future := time.Now().Add(24 * time.Hour)
	usedAt := time.Now().Add(-1 * time.Hour)
	return domain.ReconstructVerificationToken(
		tokenValue,
		domain.TypeEmailVerification,
		userID,
		future,
		&usedAt,
	)
}

// ─── NewEmailVerificationToken ────────────────────────────────────────────────

func TestNewEmailVerificationToken_InitialState(t *testing.T) {
	before := time.Now()
	token := domain.NewEmailVerificationToken("tok-abc", "user-1")
	after := time.Now()

	if token.Value() != "tok-abc" {
		t.Errorf("Value: got %q, want %q", token.Value(), "tok-abc")
	}
	if token.UserID() != "user-1" {
		t.Errorf("UserID: got %q, want %q", token.UserID(), "user-1")
	}
	if token.Type() != domain.TypeEmailVerification {
		t.Errorf("Type: got %q, want %q", token.Type(), domain.TypeEmailVerification)
	}

	// usedAt debe ser nil en un token recién creado
	if token.UsedAt() != nil {
		t.Errorf("UsedAt: got %v, want nil", token.UsedAt())
	}

	// expiresAt debe estar entre 24h después del before y 24h después del after
	minExpiry := before.Add(24 * time.Hour)
	maxExpiry := after.Add(24 * time.Hour)
	if token.ExpiresAt().Before(minExpiry) || token.ExpiresAt().After(maxExpiry) {
		t.Errorf("ExpiresAt fuera del rango esperado (24h TTL): got %v", token.ExpiresAt())
	}
}

func TestNewEmailVerificationToken_IsValidInitially(t *testing.T) {
	token := domain.NewEmailVerificationToken("tok-1", "user-1")

	if err := token.Validate(); err != nil {
		t.Errorf("token recién creado debería ser válido, got error: %v", err)
	}
}

// ─── NewPhoneOTP ──────────────────────────────────────────────────────────────

func TestNewPhoneOTP_InitialState(t *testing.T) {
	before := time.Now()
	token := domain.NewPhoneOTP("123456", "user-2")
	after := time.Now()

	if token.Value() != "123456" {
		t.Errorf("Value: got %q, want %q", token.Value(), "123456")
	}
	if token.Type() != domain.TypePhoneOTP {
		t.Errorf("Type: got %q, want %q", token.Type(), domain.TypePhoneOTP)
	}
	if token.UsedAt() != nil {
		t.Errorf("UsedAt: got %v, want nil", token.UsedAt())
	}

	// TTL de 10 minutos
	minExpiry := before.Add(10 * time.Minute)
	maxExpiry := after.Add(10 * time.Minute)
	if token.ExpiresAt().Before(minExpiry) || token.ExpiresAt().After(maxExpiry) {
		t.Errorf("ExpiresAt OTP fuera del rango esperado (10min TTL): got %v", token.ExpiresAt())
	}
}

func TestNewPhoneOTP_IsValidInitially(t *testing.T) {
	token := domain.NewPhoneOTP("654321", "user-2")

	if err := token.Validate(); err != nil {
		t.Errorf("OTP recién creado debería ser válido, got: %v", err)
	}
}

// ─── Validate ────────────────────────────────────────────────────────────────

func TestValidate_TokenValido(t *testing.T) {
	token := domain.NewEmailVerificationToken("tok", "user-1")

	if err := token.Validate(); err != nil {
		t.Errorf("Validate() token válido: got %v, want nil", err)
	}
}

func TestValidate_TokenExpirado(t *testing.T) {
	token := makeExpiredEmailToken("tok-exp", "user-1")

	err := token.Validate()
	if err == nil {
		t.Fatal("Validate() token expirado: esperaba error, got nil")
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		t.Errorf("Validate() token expirado: got %v, want %v", err, domain.ErrTokenExpired)
	}
}

func TestValidate_TokenYaUsado(t *testing.T) {
	token := makeUsedEmailToken("tok-used", "user-1")

	err := token.Validate()
	if err == nil {
		t.Fatal("Validate() token ya usado: esperaba error, got nil")
	}
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Errorf("Validate() token ya usado: got %v, want %v", err, domain.ErrTokenAlreadyUsed)
	}
}

// TestValidate_UsadoGanaASobreExpirado verifica el orden de los checks:
// usedAt se evalúa ANTES que expiresAt.
// Un token que está tanto expirado como ya usado debe retornar ErrTokenAlreadyUsed,
// no ErrTokenExpired. Esto protege contra el cambio accidental del orden en Validate().
func TestValidate_UsadoGanaASobreExpirado(t *testing.T) {
	past := time.Now().Add(-25 * time.Hour) // expirado
	usedAt := time.Now().Add(-30 * time.Hour)

	// Token expirado Y ya usado
	token := domain.ReconstructVerificationToken(
		"tok-both",
		domain.TypeEmailVerification,
		"user-1",
		past,
		&usedAt,
	)

	err := token.Validate()
	if err == nil {
		t.Fatal("Validate() token expirado+usado: esperaba error, got nil")
	}

	// El check de usedAt debe ganar
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Errorf("Validate() expirado+usado: got %v, want ErrTokenAlreadyUsed (usedAt tiene prioridad)", err)
	}
}

// ─── Use ─────────────────────────────────────────────────────────────────────

func TestUse_TokenValido_MarcaUsedAt(t *testing.T) {
	token := domain.NewEmailVerificationToken("tok", "user-1")

	before := time.Now()
	err := token.Use()
	after := time.Now()

	if err != nil {
		t.Fatalf("Use() token válido: error inesperado: %v", err)
	}

	// usedAt debe haberse seteado
	if token.UsedAt() == nil {
		t.Fatal("Use(): usedAt debe ser non-nil tras Use() exitoso")
	}

	// usedAt debe estar dentro de la ventana de ejecución del test
	if token.UsedAt().Before(before) || token.UsedAt().After(after) {
		t.Errorf("Use(): usedAt=%v fuera de la ventana [%v, %v]", token.UsedAt(), before, after)
	}
}

func TestUse_TokenValido_ValidatePostUsoFalla(t *testing.T) {
	// Verifica idempotencia: después de Use(), Validate() debe fallar con ErrTokenAlreadyUsed.
	// Esto es crítico: un token consumido no puede volver a usarse.
	token := domain.NewEmailVerificationToken("tok", "user-1")

	_ = token.Use()

	err := token.Validate()
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Errorf("Validate() post-Use(): got %v, want ErrTokenAlreadyUsed", err)
	}
}

func TestUse_TokenExpirado_RetornaError(t *testing.T) {
	token := makeExpiredEmailToken("tok-exp", "user-1")

	err := token.Use()
	if err == nil {
		t.Fatal("Use() token expirado: esperaba error, got nil")
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		t.Errorf("Use() token expirado: got %v, want ErrTokenExpired", err)
	}
}

func TestUse_TokenExpirado_UsedAtNoCambia(t *testing.T) {
	// Use() en un token expirado NO debe modificar usedAt.
	// El token permanece en su estado original.
	token := makeExpiredEmailToken("tok-exp", "user-1")

	_ = token.Use() // esperamos que falle

	if token.UsedAt() != nil {
		t.Error("Use() en token expirado no debe modificar usedAt")
	}
}

func TestUse_TokenYaUsado_RetornaError(t *testing.T) {
	// Use() en un token ya consumido debe fallar (no puede usarse dos veces).
	token := makeUsedEmailToken("tok-used", "user-1")

	err := token.Use()
	if err == nil {
		t.Fatal("Use() token ya usado: esperaba error, got nil")
	}
	if !errors.Is(err, domain.ErrTokenAlreadyUsed) {
		t.Errorf("Use() token ya usado: got %v, want ErrTokenAlreadyUsed", err)
	}
}

func TestUse_Idempotencia_UsedAtNoCambiaEnSegundoUso(t *testing.T) {
	// El primer Use() exitoso setea usedAt.
	// Un segundo Use() falla pero NO sobreescribe usedAt.
	token := domain.NewEmailVerificationToken("tok", "user-1")

	_ = token.Use() // primer uso — exitoso
	firstUsedAt := *token.UsedAt()

	// Pausa mínima para que time.Now() sea diferente si se volviera a setear
	time.Sleep(1 * time.Millisecond)

	_ = token.Use() // segundo uso — debe fallar

	// usedAt debe seguir siendo el del primer Use()
	if !token.UsedAt().Equal(firstUsedAt) {
		t.Errorf("Use() idempotencia: usedAt cambió en el segundo uso. got %v, want %v",
			token.UsedAt(), firstUsedAt)
	}
}

// ─── ReconstructVerificationToken ────────────────────────────────────────────

func TestReconstructVerificationToken_GettersDevuelvenValoresCorrectos(t *testing.T) {
	expiry := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	usedAt := time.Now().Add(-1 * time.Hour).Truncate(time.Second)

	token := domain.ReconstructVerificationToken(
		"tok-reconstruct",
		domain.TypePhoneOTP,
		"user-99",
		expiry,
		&usedAt,
	)

	if token.Value() != "tok-reconstruct" {
		t.Errorf("Value: got %q, want %q", token.Value(), "tok-reconstruct")
	}
	if token.Type() != domain.TypePhoneOTP {
		t.Errorf("Type: got %q, want %q", token.Type(), domain.TypePhoneOTP)
	}
	if token.UserID() != "user-99" {
		t.Errorf("UserID: got %q, want %q", token.UserID(), "user-99")
	}
	if !token.ExpiresAt().Equal(expiry) {
		t.Errorf("ExpiresAt: got %v, want %v", token.ExpiresAt(), expiry)
	}
	if token.UsedAt() == nil {
		t.Fatal("UsedAt: got nil, want non-nil")
	}
	if !token.UsedAt().Equal(usedAt) {
		t.Errorf("UsedAt: got %v, want %v", token.UsedAt(), usedAt)
	}
}

func TestReconstructVerificationToken_UsedAtNil(t *testing.T) {
	// Caso más común en producción: token recién guardado, aún no usado
	token := domain.ReconstructVerificationToken(
		"tok-nil",
		domain.TypeEmailVerification,
		"user-1",
		time.Now().Add(24*time.Hour),
		nil,
	)

	if token.UsedAt() != nil {
		t.Errorf("UsedAt: got %v, want nil", token.UsedAt())
	}
}

// ─── TTL diferenciado por tipo de token ──────────────────────────────────────

// TestTTLDiferenciado verifica que email y OTP tienen TTLs distintos,
// documentando el contrato de negocio: email = 24h, OTP = 10min.
func TestTTLDiferenciado(t *testing.T) {
	emailToken := domain.NewEmailVerificationToken("e", "u")
	otpToken := domain.NewPhoneOTP("o", "u")

	emailTTL := time.Until(emailToken.ExpiresAt())
	otpTTL := time.Until(otpToken.ExpiresAt())

	// Email debe expirar en ~24h (con 1s de margen por ejecución del test)
	if emailTTL < 23*time.Hour+59*time.Minute {
		t.Errorf("Email TTL demasiado corto: %v (want ~24h)", emailTTL)
	}
	if emailTTL > 24*time.Hour+1*time.Second {
		t.Errorf("Email TTL demasiado largo: %v (want ~24h)", emailTTL)
	}

	// OTP debe expirar en ~10min
	if otpTTL < 9*time.Minute+59*time.Second {
		t.Errorf("OTP TTL demasiado corto: %v (want ~10min)", otpTTL)
	}
	if otpTTL > 10*time.Minute+1*time.Second {
		t.Errorf("OTP TTL demasiado largo: %v (want ~10min)", otpTTL)
	}

	// El TTL del email debe ser significativamente mayor al del OTP
	if emailTTL <= otpTTL {
		t.Errorf("Email TTL (%v) debería ser mayor que OTP TTL (%v)", emailTTL, otpTTL)
	}
}
