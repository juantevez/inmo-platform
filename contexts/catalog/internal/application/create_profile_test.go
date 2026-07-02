package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeProfileRepo struct {
	findByIDFn      func(ctx context.Context, userID string) (*domain.Profile, error)
	findByDniCuitFn func(ctx context.Context, dniCuit string) (*domain.Profile, error)
	saveErr         error
	saved           *domain.Profile
}

func (f *fakeProfileRepo) Save(ctx context.Context, profile *domain.Profile) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = profile
	return nil
}
func (f *fakeProfileRepo) FindByID(ctx context.Context, userID string) (*domain.Profile, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, userID)
	}
	return nil, nil
}
func (f *fakeProfileRepo) FindByDniCuit(ctx context.Context, dniCuit string) (*domain.Profile, error) {
	if f.findByDniCuitFn != nil {
		return f.findByDniCuitFn(ctx, dniCuit)
	}
	return nil, nil
}

// ─── GetByUserID ────────────────────────────────────────────────────────────

func TestGetByUserID_ErrorDelRepositorio_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakeProfileRepo{
		findByIDFn: func(ctx context.Context, userID string) (*domain.Profile, error) { return nil, boom },
	}
	uc := application.NewCreateProfileUseCase(repo)

	_, err := uc.GetByUserID(context.Background(), "user-1")

	if !errors.Is(err, boom) {
		t.Fatalf("GetByUserID: got %v, want error que envuelva %v", err, boom)
	}
}

func TestGetByUserID_NoExiste_RetornaNilSinError(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	profile, err := uc.GetByUserID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("GetByUserID: error inesperado: %v", err)
	}
	if profile != nil {
		t.Errorf("GetByUserID: got %+v, want nil", profile)
	}
}

func TestGetByUserID_HappyPath_MapeaTodosLosCampos(t *testing.T) {
	domainProfile, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "+541112345678",
		domain.ProfileTypeCommercial, "Inmobiliaria Perez", "MAT-123")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	repo := &fakeProfileRepo{
		findByIDFn: func(ctx context.Context, userID string) (*domain.Profile, error) { return domainProfile, nil },
	}
	uc := application.NewCreateProfileUseCase(repo)

	dto, err := uc.GetByUserID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("GetByUserID: error inesperado: %v", err)
	}
	if dto.UserID != "user-1" || dto.FirstName != "Juan" || dto.LastName != "Perez" ||
		dto.Phone != "+541112345678" || dto.ProfileType != "COMMERCIAL" ||
		dto.CompanyName != "Inmobiliaria Perez" || dto.LicenseNumber != "MAT-123" ||
		dto.Status != "PENDING_VERIFICATION" {
		t.Errorf("ProfileDTO: got %+v", dto)
	}
}

// ─── Execute ────────────────────────────────────────────────────────────────

func TestCreateProfile_ErrorVerificandoDniCuit_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakeProfileRepo{
		findByDniCuitFn: func(ctx context.Context, dniCuit string) (*domain.Profile, error) { return nil, boom },
	}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-12345678-9",
		ProfileType: "INDIVIDUAL",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestCreateProfile_DniCuitDeOtroUsuario_RetornaErrDniCuitAlreadyExists(t *testing.T) {
	existing, err := domain.NewProfile("otro-user", "Ana", "Gomez", "20-99999999-9", "", domain.ProfileTypeIndividual, "", "")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	repo := &fakeProfileRepo{
		findByDniCuitFn: func(ctx context.Context, dniCuit string) (*domain.Profile, error) { return existing, nil },
	}
	uc := application.NewCreateProfileUseCase(repo)

	execErr := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-99999999-9",
		ProfileType: "INDIVIDUAL",
	})

	if !errors.Is(execErr, application.ErrDniCuitAlreadyExists) {
		t.Fatalf("Execute: got %v, want ErrDniCuitAlreadyExists", execErr)
	}
}

func TestCreateProfile_DniCuitDelMismoUsuario_PermiteActualizar(t *testing.T) {
	// Si el DNI/CUIT ya existe pero pertenece al mismo usuario que hace el request,
	// es una actualización legítima — no debe bloquearse como conflicto.
	existing, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "", domain.ProfileTypeIndividual, "", "")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	repo := &fakeProfileRepo{
		findByDniCuitFn: func(ctx context.Context, dniCuit string) (*domain.Profile, error) { return existing, nil },
	}
	uc := application.NewCreateProfileUseCase(repo)

	execErr := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez Actualizado", DniCuit: "20-12345678-9",
		ProfileType: "INDIVIDUAL",
	})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if repo.saved == nil || repo.saved.LastName() != "Perez Actualizado" {
		t.Errorf("Save: got %+v, want el perfil actualizado", repo.saved)
	}
}

func TestCreateProfile_CamposObligatoriosFaltantes_RetornaErrorDeDominioEnvuelto(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", ProfileType: "INDIVIDUAL", // sin FirstName/LastName/DniCuit
	})

	if !errors.Is(err, domain.ErrMissingRequiredFields) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, domain.ErrMissingRequiredFields)
	}
}

func TestCreateProfile_TipoDePerfilInvalido_RetornaErrorDeDominioEnvuelto(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-12345678-9",
		ProfileType: "SUPERADMIN",
	})

	if !errors.Is(err, domain.ErrInvalidProfileType) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, domain.ErrInvalidProfileType)
	}
}

func TestCreateProfile_ComercialSinDatosDeEmpresa_RetornaErrorDeDominioEnvuelto(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-12345678-9",
		ProfileType: "COMMERCIAL", // sin CompanyName/LicenseNumber
	})

	if !errors.Is(err, domain.ErrCommercialMissingData) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, domain.ErrCommercialMissingData)
	}
}

func TestCreateProfile_ErrorAlPersistir_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("fallo de escritura en Postgres")
	repo := &fakeProfileRepo{saveErr: boom}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-12345678-9",
		ProfileType: "INDIVIDUAL",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestCreateProfile_HappyPath_Individual(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "20-12345678-9",
		Phone: "+541112345678", ProfileType: "INDIVIDUAL",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("Save: no fue invocado")
	}
	if repo.saved.UserID() != "user-1" || repo.saved.ProfileType() != domain.ProfileTypeIndividual {
		t.Errorf("Save: got UserID=%q ProfileType=%q", repo.saved.UserID(), repo.saved.ProfileType())
	}
	if repo.saved.Status() != domain.StatusPending {
		t.Errorf("Save Status: got %q, want %q (todo perfil nuevo arranca pendiente)", repo.saved.Status(), domain.StatusPending)
	}
}

func TestCreateProfile_HappyPath_Comercial(t *testing.T) {
	repo := &fakeProfileRepo{}
	uc := application.NewCreateProfileUseCase(repo)

	err := uc.Execute(context.Background(), application.CreateProfileCommand{
		UserID: "user-1", FirstName: "Juan", LastName: "Perez", DniCuit: "30-12345678-9",
		ProfileType: "COMMERCIAL", CompanyName: "Inmobiliaria Perez", LicenseNumber: "MAT-123",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.saved.CompanyName() != "Inmobiliaria Perez" || repo.saved.LicenseNumber() != "MAT-123" {
		t.Errorf("Save: got CompanyName=%q LicenseNumber=%q", repo.saved.CompanyName(), repo.saved.LicenseNumber())
	}
}
