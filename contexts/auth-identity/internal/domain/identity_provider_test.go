package domain_test

import (
	"strings"
	"sync"
	"testing"

	"inmo.platform/contexts/auth-identity/internal/domain"
)

// ─── fixture compartida para tests de bcrypt ─────────────────────────────────
//
// NewEmailProvider hashea con bcrypt costo 12 (~300ms).
// Usar sync.Once garantiza que el hash se computa UNA SOLA VEZ para todo
// el paquete de tests, evitando que el suite tarde segundos innecesarios.

var (
	sharedEmailProvider     *domain.IdentityProvider
	sharedEmailProviderOnce sync.Once
	sharedEmailProviderErr  error
)

const (
	sharedUserID   = "user-fixture-001"
	sharedEmail    = "fixture@test.com"
	sharedPassword = "Password1"
	wrongPassword  = "WrongPassword99"
)

func getSharedEmailProvider(t *testing.T) *domain.IdentityProvider {
	t.Helper()
	sharedEmailProviderOnce.Do(func() {
		sharedEmailProvider, sharedEmailProviderErr = domain.NewEmailProvider(
			"prov-fixture", sharedUserID, sharedEmail, sharedPassword,
		)
	})
	if sharedEmailProviderErr != nil {
		t.Fatalf("getSharedEmailProvider: %v", sharedEmailProviderErr)
	}
	return sharedEmailProvider
}

// ─── NewEmailProvider ─────────────────────────────────────────────────────────

func TestNewEmailProvider_PasswordDemasiadoCorta(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"vacía", ""},
		{"1 char", "a"},
		{"7 chars", "Pass12!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewEmailProvider("id", "uid", "u@test.com", tc.password)
			if err == nil {
				t.Errorf("NewEmailProvider(%q): esperaba error por password corta, got nil", tc.password)
			}
		})
	}
}

func TestNewEmailProvider_HappyPath_CamposCorrectos(t *testing.T) {
	p := getSharedEmailProvider(t)

	if p.ID() != "prov-fixture" {
		t.Errorf("ID: got %q, want %q", p.ID(), "prov-fixture")
	}
	if p.Name() != domain.ProviderEmail {
		t.Errorf("Name: got %q, want %q", p.Name(), domain.ProviderEmail)
	}
	// Para EMAIL, providerUserID es el email del usuario
	if p.ProviderUserID() != sharedEmail {
		t.Errorf("ProviderUserID: got %q, want %q", p.ProviderUserID(), sharedEmail)
	}
}

func TestNewEmailProvider_PasswordHashNoEsPlaintext(t *testing.T) {
	p := getSharedEmailProvider(t)

	// El hash nunca debe contener la contraseña en texto plano
	if strings.Contains(p.PasswordHash(), sharedPassword) {
		t.Error("PasswordHash contiene la contraseña en texto plano — bcrypt no se aplicó")
	}

	// El hash de bcrypt siempre empieza con "$2a$" o "$2b$"
	hash := p.PasswordHash()
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("PasswordHash no tiene formato bcrypt válido: %q", hash[:min(len(hash), 10)])
	}
}

func TestNewEmailProvider_PasswordDe8Chars_EsValida(t *testing.T) {
	// Boundary: exactamente 8 caracteres debe ser aceptado
	_, err := domain.NewEmailProvider("id", "uid", "u@test.com", "Passw0rd")
	if err != nil {
		t.Errorf("password de exactamente 8 chars: error inesperado: %v", err)
	}
}

// ─── NewSSOProvider ───────────────────────────────────────────────────────────

func TestNewSSOProvider_TipoInvalido(t *testing.T) {
	_, err := domain.NewSSOProvider("id", "uid", domain.ProviderEmail, "external-id")
	if err == nil {
		t.Error("NewSSOProvider con ProviderEmail: esperaba error, got nil")
	}
}

func TestNewSSOProvider_ProviderUserIDVacio(t *testing.T) {
	_, err := domain.NewSSOProvider("id", "uid", domain.ProviderGoogle, "")
	if err == nil {
		t.Error("NewSSOProvider con providerUserID vacío: esperaba error, got nil")
	}
}

func TestNewSSOProvider_Google_HappyPath(t *testing.T) {
	p, err := domain.NewSSOProvider("prov-g", "user-1", domain.ProviderGoogle, "google-sub-123")
	if err != nil {
		t.Fatalf("NewSSOProvider Google: error inesperado: %v", err)
	}

	if p.ID() != "prov-g" {
		t.Errorf("ID: got %q, want %q", p.ID(), "prov-g")
	}
	if p.Name() != domain.ProviderGoogle {
		t.Errorf("Name: got %q, want %q", p.Name(), domain.ProviderGoogle)
	}
	if p.ProviderUserID() != "google-sub-123" {
		t.Errorf("ProviderUserID: got %q, want %q", p.ProviderUserID(), "google-sub-123")
	}
	// SSO no tiene password — hash debe ser vacío
	if p.PasswordHash() != "" {
		t.Errorf("PasswordHash SSO: got %q, want empty string", p.PasswordHash())
	}
}

func TestNewSSOProvider_Meta_HappyPath(t *testing.T) {
	p, err := domain.NewSSOProvider("prov-m", "user-1", domain.ProviderMeta, "meta-id-456")
	if err != nil {
		t.Fatalf("NewSSOProvider Meta: error inesperado: %v", err)
	}

	if p.Name() != domain.ProviderMeta {
		t.Errorf("Name: got %q, want %q", p.Name(), domain.ProviderMeta)
	}
	if p.ProviderUserID() != "meta-id-456" {
		t.Errorf("ProviderUserID: got %q, want %q", p.ProviderUserID(), "meta-id-456")
	}
}

// ─── VerifyPassword ───────────────────────────────────────────────────────────

func TestVerifyPassword_Correcta(t *testing.T) {
	p := getSharedEmailProvider(t)

	if !p.VerifyPassword(sharedPassword) {
		t.Error("VerifyPassword con contraseña correcta: got false, want true")
	}
}

func TestVerifyPassword_Incorrecta(t *testing.T) {
	p := getSharedEmailProvider(t)

	if p.VerifyPassword(wrongPassword) {
		t.Error("VerifyPassword con contraseña incorrecta: got true, want false")
	}
}

func TestVerifyPassword_Vacia(t *testing.T) {
	p := getSharedEmailProvider(t)

	if p.VerifyPassword("") {
		t.Error("VerifyPassword con contraseña vacía: got true, want false")
	}
}

func TestVerifyPassword_ProveedorSSO_SiempreFalse(t *testing.T) {
	// Un provider SSO no tiene contraseña — VerifyPassword debe retornar false
	// inmediatamente sin intentar comparar con bcrypt (hash está vacío).
	// Este test también verifica que no paniquea con hash vacío.
	googleProv, _ := domain.NewSSOProvider("prov-g", "uid", domain.ProviderGoogle, "sub-123")

	if googleProv.VerifyPassword("cualquiercosa") {
		t.Error("VerifyPassword en provider Google: got true, want false")
	}
	if googleProv.VerifyPassword("") {
		t.Error("VerifyPassword vacía en provider Google: got true, want false")
	}
}

func TestVerifyPassword_ProveedorMeta_SiempreFalse(t *testing.T) {
	metaProv, _ := domain.NewSSOProvider("prov-m", "uid", domain.ProviderMeta, "meta-456")

	if metaProv.VerifyPassword(sharedPassword) {
		t.Error("VerifyPassword en provider Meta: got true, want false")
	}
}

// ─── ReconstructProvider ─────────────────────────────────────────────────────

func TestReconstructProvider_GettersDevuelvenValoresCorrectos(t *testing.T) {
	p := domain.ReconstructProvider(
		"prov-99",
		"user-99",
		domain.ProviderGoogle,
		"google-sub-xyz",
		"",
	)

	cases := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"ID", p.ID(), "prov-99"},
		{"Name", p.Name(), domain.ProviderGoogle},
		{"ProviderUserID", p.ProviderUserID(), "google-sub-xyz"},
		{"PasswordHash", p.PasswordHash(), ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestReconstructProvider_ConPasswordHash(t *testing.T) {
	// ReconstructProvider es usado por el repositorio para rehidratar
	// providers EMAIL desde la DB — el hash viene de la columna password_hash.
	const storedHash = "$2a$12$fakehashfortest"

	p := domain.ReconstructProvider(
		"prov-email",
		"user-1",
		domain.ProviderEmail,
		"user@test.com",
		storedHash,
	)

	if p.PasswordHash() != storedHash {
		t.Errorf("PasswordHash: got %q, want %q", p.PasswordHash(), storedHash)
	}
	if p.Name() != domain.ProviderEmail {
		t.Errorf("Name: got %q, want %q", p.Name(), domain.ProviderEmail)
	}
}

// ─── SetPasswordHash ──────────────────────────────────────────────────────────

func TestSetPasswordHash_ActualizaElHash(t *testing.T) {
	p := domain.ReconstructProvider(
		"prov-1", "user-1", domain.ProviderEmail, "u@test.com", "hash-viejo",
	)

	p.SetPasswordHash("hash-nuevo")

	if p.PasswordHash() != "hash-nuevo" {
		t.Errorf("PasswordHash post-Set: got %q, want %q", p.PasswordHash(), "hash-nuevo")
	}
}

// ─── Tipos de provider — constantes ──────────────────────────────────────────

// TestProviderTypeConstants verifica que los valores de las constantes
// coinciden con lo que la base de datos espera en la columna provider_name.
// Si alguien renombra una constante sin actualizar las migraciones SQL, este test falla.
func TestProviderTypeConstants(t *testing.T) {
	cases := []struct {
		name     string
		got      domain.ProviderType
		expected string
	}{
		{"EMAIL", domain.ProviderEmail, "EMAIL"},
		{"GOOGLE", domain.ProviderGoogle, "GOOGLE"},
		{"META", domain.ProviderMeta, "META"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.expected {
				t.Errorf("ProviderType %s: got %q, want %q", tc.name, tc.got, tc.expected)
			}
		})
	}
}
