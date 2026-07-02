package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── NewProfile ─────────────────────────────────────────────────────────────

func TestNewProfile_CamposObligatoriosFaltantes(t *testing.T) {
	cases := []struct {
		name                                        string
		userID, firstName, lastName, dniCuit, phone string
	}{
		{"userID vacío", "", "Juan", "Perez", "20-12345678-9", ""},
		{"firstName vacío", "user-1", "", "Perez", "20-12345678-9", ""},
		{"lastName vacío", "user-1", "Juan", "", "20-12345678-9", ""},
		{"dniCuit vacío", "user-1", "Juan", "Perez", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewProfile(tc.userID, tc.firstName, tc.lastName, tc.dniCuit, tc.phone, domain.ProfileTypeIndividual, "", "")
			if !errors.Is(err, domain.ErrMissingRequiredFields) {
				t.Fatalf("NewProfile: got %v, want %v", err, domain.ErrMissingRequiredFields)
			}
		})
	}
}

func TestNewProfile_TipoInvalido_RetornaErrInvalidProfileType(t *testing.T) {
	_, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "", domain.ProfileType("SUPERADMIN"), "", "")

	if !errors.Is(err, domain.ErrInvalidProfileType) {
		t.Fatalf("NewProfile: got %v, want %v", err, domain.ErrInvalidProfileType)
	}
}

func TestNewProfile_ComercialSinCompanyName_RetornaErrCommercialMissingData(t *testing.T) {
	_, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "", domain.ProfileTypeCommercial, "", "MAT-123")

	if !errors.Is(err, domain.ErrCommercialMissingData) {
		t.Fatalf("NewProfile: got %v, want %v", err, domain.ErrCommercialMissingData)
	}
}

func TestNewProfile_ComercialSinLicenseNumber_RetornaErrCommercialMissingData(t *testing.T) {
	_, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "", domain.ProfileTypeCommercial, "Inmobiliaria Perez", "")

	if !errors.Is(err, domain.ErrCommercialMissingData) {
		t.Fatalf("NewProfile: got %v, want %v", err, domain.ErrCommercialMissingData)
	}
}

func TestNewProfile_Individual_NoExigeDatosComerciales(t *testing.T) {
	// Un perfil INDIVIDUAL nunca dispara la validación de company_name/license_number,
	// incluso si vienen vacíos (son campos exclusivos de COMMERCIAL).
	profile, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "+541112345678", domain.ProfileTypeIndividual, "", "")

	if err != nil {
		t.Fatalf("NewProfile: error inesperado: %v", err)
	}
	if profile.CompanyName() != "" || profile.LicenseNumber() != "" {
		t.Errorf("got CompanyName=%q LicenseNumber=%q, want ambos vacíos", profile.CompanyName(), profile.LicenseNumber())
	}
}

func TestNewProfile_HappyPath_Comercial(t *testing.T) {
	before := time.Now()
	profile, err := domain.NewProfile("user-1", "Juan", "Perez", "30-12345678-9", "+541112345678",
		domain.ProfileTypeCommercial, "Inmobiliaria Perez", "MAT-123")
	after := time.Now()

	if err != nil {
		t.Fatalf("NewProfile: error inesperado: %v", err)
	}
	if profile.UserID() != "user-1" || profile.FirstName() != "Juan" || profile.LastName() != "Perez" ||
		profile.DniCuit() != "30-12345678-9" || profile.Phone() != "+541112345678" ||
		profile.ProfileType() != domain.ProfileTypeCommercial ||
		profile.CompanyName() != "Inmobiliaria Perez" || profile.LicenseNumber() != "MAT-123" {
		t.Errorf("profile: got %+v", profile)
	}
	if profile.Status() != domain.StatusPending {
		t.Errorf("Status: got %q, want %q (todo perfil nuevo arranca pendiente de verificación)", profile.Status(), domain.StatusPending)
	}
	if profile.CreatedAt().Before(before) || profile.CreatedAt().After(after) {
		t.Errorf("CreatedAt: got %v, want entre %v y %v", profile.CreatedAt(), before, after)
	}
	if !profile.CreatedAt().Equal(profile.UpdatedAt()) {
		t.Errorf("CreatedAt/UpdatedAt: got %v / %v, want iguales en la creación", profile.CreatedAt(), profile.UpdatedAt())
	}
}

// ─── ReconstructProfile ─────────────────────────────────────────────────────

func TestReconstructProfile_BypaseaValidacionesYPreservaValoresCrudos(t *testing.T) {
	createdAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Reconstruct permite estados/valores que NewProfile jamás aceptaría —
	// necesario para hidratar tal cual desde Postgres.
	profile := domain.ReconstructProfile("user-1", "", "", "", "", domain.ProfileType("LEGACY"), "", "",
		domain.StatusSuspended, createdAt, updatedAt)

	if profile.UserID() != "user-1" {
		t.Errorf("UserID: got %q", profile.UserID())
	}
	if profile.ProfileType() != domain.ProfileType("LEGACY") {
		t.Errorf("ProfileType: got %q, want se preserve sin validar", profile.ProfileType())
	}
	if profile.Status() != domain.StatusSuspended {
		t.Errorf("Status: got %q, want %q", profile.Status(), domain.StatusSuspended)
	}
	if !profile.CreatedAt().Equal(createdAt) || !profile.UpdatedAt().Equal(updatedAt) {
		t.Errorf("timestamps: got CreatedAt=%v UpdatedAt=%v, want los valores originales", profile.CreatedAt(), profile.UpdatedAt())
	}
}
