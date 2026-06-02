package domain_test

import (
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/domain"
)

// ─── NewUser ──────────────────────────────────────────────────────────────────

func TestNewUser_InvalidEmail(t *testing.T) {
	// El dominio usa strings.Contains(email, "@") como validación básica
	// (ver comentario en user.go: "después se puede usar regex").
	// Solo los casos que NO contienen "@" son rechazados por esta validación.
	cases := []struct {
		name  string
		email string
	}{
		{"sin arroba", "notanemail"},
		{"vacío", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewUser("id-1", tc.email)
			if err == nil {
				t.Errorf("NewUser(%q): esperaba error, got nil", tc.email)
			}
		})
	}
}

func TestNewUser_ValidEmail_InitialState(t *testing.T) {
	user, err := domain.NewUser("user-001", "Test@Example.COM")
	if err != nil {
		t.Fatalf("NewUser: error inesperado: %v", err)
	}

	// Nace en PENDING_VERIFICATION
	if user.Status() != domain.StatusPendingVerification {
		t.Errorf("Status: got %q, want %q", user.Status(), domain.StatusPendingVerification)
	}

	// Email normalizado a lowercase y sin espacios
	if user.Email() != "test@example.com" {
		t.Errorf("Email normalizado: got %q, want %q", user.Email(), "test@example.com")
	}

	// ID preservado tal cual
	if user.ID() != "user-001" {
		t.Errorf("ID: got %q, want %q", user.ID(), "user-001")
	}

	// Providers inicializado vacío (no nil)
	if user.Providers() == nil {
		t.Error("Providers() no debe ser nil en un usuario nuevo")
	}
	if len(user.Providers()) != 0 {
		t.Errorf("Providers() len: got %d, want 0", len(user.Providers()))
	}

	// CreatedAt seteado (no zero value)
	if user.CreatedAt().IsZero() {
		t.Error("CreatedAt() no debe ser zero en un usuario nuevo")
	}
}

func TestNewUser_EmailNormalization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"  user@domain.com  ", "user@domain.com"},
		{"USER@DOMAIN.COM", "user@domain.com"},
		{"Mixed@Case.Org", "mixed@case.org"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			user, err := domain.NewUser("id", tc.input)
			if err != nil {
				t.Fatalf("NewUser(%q): error inesperado: %v", tc.input, err)
			}
			if user.Email() != tc.expected {
				t.Errorf("Email: got %q, want %q", user.Email(), tc.expected)
			}
		})
	}
}

// ─── NewUserFromSSO ───────────────────────────────────────────────────────────

func TestNewUserFromSSO_InvalidEmail(t *testing.T) {
	_, err := domain.NewUserFromSSO("id-1", "bademail")
	if err == nil {
		t.Error("NewUserFromSSO con email inválido: esperaba error, got nil")
	}
}

func TestNewUserFromSSO_NacesActivo(t *testing.T) {
	user, err := domain.NewUserFromSSO("id-1", "google@user.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: error inesperado: %v", err)
	}

	// SSO no requiere verificación de email — nace ACTIVE directamente
	if user.Status() != domain.StatusActive {
		t.Errorf("Status: got %q, want %q", user.Status(), domain.StatusActive)
	}
}

// ─── Activate ────────────────────────────────────────────────────────────────

func TestActivate_DesdePending_PasaAActive(t *testing.T) {
	user, _ := domain.NewUser("id-1", "user@test.com")
	// Nace en PENDING_VERIFICATION

	err := user.Activate()
	if err != nil {
		t.Fatalf("Activate: error inesperado: %v", err)
	}

	if user.Status() != domain.StatusActive {
		t.Errorf("Status post-Activate: got %q, want %q", user.Status(), domain.StatusActive)
	}
}

func TestActivate_DesdeActive_RetornaError(t *testing.T) {
	user, _ := domain.NewUserFromSSO("id-1", "user@test.com")
	// SSO → nace ACTIVE

	err := user.Activate()
	if err == nil {
		t.Error("Activate sobre usuario ya activo: esperaba error, got nil")
	}
}

func TestActivate_DesdeSuspended_RetornaError(t *testing.T) {
	// ReconstructUser permite construir usuarios en cualquier estado
	// sin pasar por las validaciones de la fábrica — igual que lo haría
	// la capa de infraestructura al hidratar desde la base de datos.
	suspended := domain.ReconstructUser(
		"id-1", "user@test.com",
		string(domain.StatusSuspended),
		"", nil, time.Now(),
	)

	err := suspended.Activate()
	if err == nil {
		t.Error("Activate sobre usuario suspendido: esperaba error, got nil")
	}
}

// ─── LinkProvider ─────────────────────────────────────────────────────────────

// buildEmailProvider construye un proveedor EMAIL mínimo para los tests de LinkProvider.
func buildEmailProvider(t *testing.T) *domain.IdentityProvider {
	t.Helper()
	p, err := domain.NewEmailProvider("prov-id", "user-id", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("buildEmailProvider: %v", err)
	}
	return p
}

// buildGoogleProvider construye un proveedor GOOGLE mínimo.
func buildGoogleProvider(t *testing.T, providerID, userID string) *domain.IdentityProvider {
	t.Helper()
	p, err := domain.NewSSOProvider(providerID, userID, domain.ProviderGoogle, "google-uid-123")
	if err != nil {
		t.Fatalf("buildGoogleProvider: %v", err)
	}
	return p
}

func TestLinkProvider_UsuarioSuspendido_RetornaError(t *testing.T) {
	suspended := domain.ReconstructUser(
		"id-1", "user@test.com",
		string(domain.StatusSuspended),
		"", nil, time.Now(),
	)

	emailProv := buildEmailProvider(t)
	err := suspended.LinkProvider(emailProv)
	if err == nil {
		t.Error("LinkProvider sobre suspendido: esperaba error, got nil")
	}
}

func TestLinkProvider_Pending_ConProveedorSSO_RetornaError(t *testing.T) {
	// Un usuario PENDING no puede vincular SSO — debe verificar el email primero.
	user, _ := domain.NewUser("id-1", "user@test.com")
	// Status = PENDING_VERIFICATION

	googleProv := buildGoogleProvider(t, "prov-id", "id-1")
	err := user.LinkProvider(googleProv)
	if err == nil {
		t.Error("LinkProvider SSO sobre PENDING: esperaba error, got nil")
	}
}

func TestLinkProvider_Pending_ConProveedorEmail_Permitido(t *testing.T) {
	// El flujo de registro normal: usuario PENDING + provider EMAIL está permitido.
	user, _ := domain.NewUser("id-1", "user@test.com")

	emailProv := buildEmailProvider(t)
	err := user.LinkProvider(emailProv)
	if err != nil {
		t.Errorf("LinkProvider EMAIL sobre PENDING: error inesperado: %v", err)
	}

	if len(user.Providers()) != 1 {
		t.Errorf("Providers len: got %d, want 1", len(user.Providers()))
	}
}

func TestLinkProvider_ProviderDuplicado_RetornaError(t *testing.T) {
	// Un usuario ACTIVE no puede vincular el mismo provider dos veces.
	user, _ := domain.NewUserFromSSO("id-1", "user@test.com")
	// Status = ACTIVE

	googleProv1 := buildGoogleProvider(t, "prov-1", "id-1")
	googleProv2 := buildGoogleProvider(t, "prov-2", "id-1") // mismo tipo, distinto ID

	_ = user.LinkProvider(googleProv1)

	err := user.LinkProvider(googleProv2)
	if err == nil {
		t.Error("LinkProvider duplicado: esperaba error, got nil")
	}
}

func TestLinkProvider_MultipleProviders_Diferentes(t *testing.T) {
	// Un usuario ACTIVE puede tener múltiples providers de tipos distintos.
	user, _ := domain.NewUserFromSSO("id-1", "user@test.com")

	googleProv, _ := domain.NewSSOProvider("prov-g", "id-1", domain.ProviderGoogle, "google-uid")
	metaProv, _ := domain.NewSSOProvider("prov-m", "id-1", domain.ProviderMeta, "meta-uid")

	if err := user.LinkProvider(googleProv); err != nil {
		t.Fatalf("LinkProvider Google: %v", err)
	}
	if err := user.LinkProvider(metaProv); err != nil {
		t.Fatalf("LinkProvider Meta: %v", err)
	}

	if len(user.Providers()) != 2 {
		t.Errorf("Providers len: got %d, want 2", len(user.Providers()))
	}
}

func TestLinkProvider_HappyPath_AgregarGoogleAActive(t *testing.T) {
	// Escenario C del gateway: usuario con cuenta local que vincula Google.
	user, _ := domain.NewUserFromSSO("id-1", "user@test.com")

	googleProv := buildGoogleProvider(t, "prov-id", "id-1")
	err := user.LinkProvider(googleProv)
	if err != nil {
		t.Fatalf("LinkProvider: error inesperado: %v", err)
	}

	providers := user.Providers()
	if len(providers) != 1 {
		t.Fatalf("Providers len: got %d, want 1", len(providers))
	}
	if providers[0].Name() != domain.ProviderGoogle {
		t.Errorf("Provider[0].Name(): got %q, want %q", providers[0].Name(), domain.ProviderGoogle)
	}
}

// ─── ReconstructUser ──────────────────────────────────────────────────────────

func TestReconstructUser_GettersDevuelvenValoresCorrectos(t *testing.T) {
	// ReconstructUser es la fábrica de infraestructura — bypasea validaciones
	// y permite estados arbitrarios (necesario para rehidratar desde DB).
	now := time.Now().Truncate(time.Second)
	verifiedAt := now.Add(-1 * time.Hour)

	user := domain.ReconstructUser(
		"user-999",
		"restored@test.com",
		string(domain.StatusActive),
		"+541112345678",
		&verifiedAt,
		now,
	)

	cases := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"ID", user.ID(), "user-999"},
		{"Email", user.Email(), "restored@test.com"},
		{"Status", user.Status(), domain.StatusActive},
		{"Phone", user.Phone(), "+541112345678"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}

	// PhoneVerifiedAt: verifica que el puntero no es nil y apunta al tiempo correcto
	if user.PhoneVerifiedAt() == nil {
		t.Fatal("PhoneVerifiedAt: got nil, want non-nil")
	}
	if !user.PhoneVerifiedAt().Equal(verifiedAt) {
		t.Errorf("PhoneVerifiedAt: got %v, want %v", user.PhoneVerifiedAt(), verifiedAt)
	}

	// CreatedAt
	if !user.CreatedAt().Equal(now) {
		t.Errorf("CreatedAt: got %v, want %v", user.CreatedAt(), now)
	}
}

func TestReconstructUser_PhoneVerifiedAtNil(t *testing.T) {
	// Caso común: usuario sin teléfono verificado
	user := domain.ReconstructUser(
		"user-1", "user@test.com",
		string(domain.StatusPendingVerification),
		"", nil, time.Now(),
	)

	if user.PhoneVerifiedAt() != nil {
		t.Errorf("PhoneVerifiedAt: got %v, want nil", user.PhoneVerifiedAt())
	}
}

// ─── Máquina de estados — transiciones completas ──────────────────────────────

// TestUserStateMachine verifica todas las transiciones válidas e inválidas
// del ciclo de vida del agregado User en una tabla compacta.
func TestUserStateMachine(t *testing.T) {
	t.Run("PENDING → ACTIVE vía Activate()", func(t *testing.T) {
		user, _ := domain.NewUser("id", "u@test.com")
		if err := user.Activate(); err != nil {
			t.Errorf("transición válida PENDING→ACTIVE: %v", err)
		}
		if user.Status() != domain.StatusActive {
			t.Errorf("estado final: got %q, want ACTIVE", user.Status())
		}
	})

	t.Run("ACTIVE → ACTIVE vía Activate() es error", func(t *testing.T) {
		user, _ := domain.NewUserFromSSO("id", "u@test.com")
		if err := user.Activate(); err == nil {
			t.Error("transición inválida ACTIVE→ACTIVE debería fallar")
		}
	})

	t.Run("SUSPENDED → ACTIVE vía Activate() es error", func(t *testing.T) {
		user := domain.ReconstructUser("id", "u@test.com",
			string(domain.StatusSuspended), "", nil, time.Now())
		if err := user.Activate(); err == nil {
			t.Error("transición inválida SUSPENDED→ACTIVE debería fallar")
		}
	})

	t.Run("SUSPENDED bloquea LinkProvider", func(t *testing.T) {
		user := domain.ReconstructUser("id", "u@test.com",
			string(domain.StatusSuspended), "", nil, time.Now())
		p, _ := domain.NewEmailProvider("p", "id", "u@test.com", "Password1")
		if err := user.LinkProvider(p); err == nil {
			t.Error("LinkProvider sobre SUSPENDED debería fallar")
		}
	})

	t.Run("PENDING bloquea SSO pero permite EMAIL", func(t *testing.T) {
		user, _ := domain.NewUser("id", "u@test.com")

		// SSO debe fallar
		g, _ := domain.NewSSOProvider("p1", "id", domain.ProviderGoogle, "uid")
		if err := user.LinkProvider(g); err == nil {
			t.Error("LinkProvider Google sobre PENDING debería fallar")
		}

		// EMAIL debe funcionar
		e, _ := domain.NewEmailProvider("p2", "id", "u@test.com", "Password1")
		if err := user.LinkProvider(e); err != nil {
			t.Errorf("LinkProvider EMAIL sobre PENDING no debería fallar: %v", err)
		}
	})
}
