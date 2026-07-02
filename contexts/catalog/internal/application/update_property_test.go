package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }
func intPtr(i int) *int           { return &i }

// ─── Propiedad no encontrada / error de repo ───────────────────────────────

func TestUpdateProperty_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestUpdateProperty_PropiedadNoExiste_RetornaNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "no-existe", application.UpdatePropertyDTO{})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

// ─── DTO vacío ──────────────────────────────────────────────────────────────

func TestUpdateProperty_DTOVacio_NoModificaNadaYGuarda(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	originalTitle := prop.Title()
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	if err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{}); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.Title() != originalTitle {
		t.Errorf("Title: got %q, want sin cambios (%q)", repo.savedViaTx.Title(), originalTitle)
	}
}

// ─── Título / descripción / precio ─────────────────────────────────────────

func TestUpdateProperty_ActualizaTitulo(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Title: strPtr("Depto renovado")})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.Title() != "Depto renovado" {
		t.Errorf("Title: got %q, want %q", repo.savedViaTx.Title(), "Depto renovado")
	}
}

func TestUpdateProperty_ActualizaPrecioConNuevaMoneda(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1") // creada en USD
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{
		Price: floatPtr(50000), Currency: strPtr("ARS"),
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.Price().Amount() != 50000 || repo.savedViaTx.Price().Currency() != domain.ARS {
		t.Errorf("Price: got %+v, want 50000 ARS", repo.savedViaTx.Price())
	}
}

func TestUpdateProperty_ActualizaPrecioSinMoneda_MantieneLaActual(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1") // USD
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Price: floatPtr(2000)})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.Price().Currency() != domain.USD {
		t.Errorf("Currency: got %q, want se mantenga USD", repo.savedViaTx.Price().Currency())
	}
}

func TestUpdateProperty_PrecioInvalido_RetornaErrorDeDominio(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Price: floatPtr(-500)})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (precio inválido)", err)
	}
}

func TestUpdateProperty_TituloVacio_RetornaErrorDeDominio(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Title: strPtr("")})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (título vacío)", err)
	}
}

func TestUpdateProperty_PropiedadReservada_RetornaPreconditionFailed(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Title: strPtr("Nuevo título")})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed (propiedad reservada)", err)
	}
}

// ─── Ubicación ──────────────────────────────────────────────────────────────

func TestUpdateProperty_ActualizaUbicacion(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Address: strPtr("Nueva Dirección 456")})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.Location().Address() != "Nueva Dirección 456" {
		t.Errorf("Address: got %q, want %q", repo.savedViaTx.Location().Address(), "Nueva Dirección 456")
	}
}

func TestUpdateProperty_UbicacionInvalida_RetornaErrorDeDominio(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{Latitude: floatPtr(999)})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (latitud inválida)", err)
	}
}

// ─── Política de mascotas ───────────────────────────────────────────────────

func TestUpdateProperty_ActualizaPetPolicy(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{PetPolicy: strPtr("ALLOWED")})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.PetPolicy() != domain.PetPolicyAllowed {
		t.Errorf("PetPolicy: got %q, want %q", repo.savedViaTx.PetPolicy(), domain.PetPolicyAllowed)
	}
}

func TestUpdateProperty_PetPolicyInvalida_RetornaErrorDeDominio(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{PetPolicy: strPtr("SOLO_GATOS")})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (pet_policy inválida)", err)
	}
}

// ─── TempConfig ─────────────────────────────────────────────────────────────

func buildTempPropertyForUpdate(t *testing.T) *domain.Property {
	t.Helper()
	tc, err := domain.NewTempConfig(nil, "14:00", "10:00", 1, 30, 50, 10, 100, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	return buildTempProperty(t, "prop-1", tc)
}

func TestUpdateProperty_PropiedadNoTemp_IgnoraCamposDeTempConfig(t *testing.T) {
	// buildProperty crea una propiedad SALE — los campos de TempConfig del DTO
	// no deben aplicarse porque OperationType() != TEMP.
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{NightPrice: floatPtr(999)})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.TempConfig().NightPrice() != 0 {
		t.Errorf("TempConfig.NightPrice: got %v, want 0 (no debería aplicarse en una propiedad SALE)", repo.savedViaTx.TempConfig().NightPrice())
	}
}

func TestUpdateProperty_Temp_ActualizaNightPriceYMantieneElResto(t *testing.T) {
	prop := buildTempPropertyForUpdate(t)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{NightPrice: floatPtr(75)})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	tc := repo.savedViaTx.TempConfig()
	if tc.NightPrice() != 75 {
		t.Errorf("NightPrice: got %v, want 75", tc.NightPrice())
	}
	// El resto de los campos de TempConfig deben conservarse sin cambios.
	if tc.CleaningFee() != 10 || tc.SecurityDeposit() != 100 || tc.MinNights() != 1 || tc.MaxNights() != 30 {
		t.Errorf("TempConfig: otros campos cambiaron inesperadamente: got %+v", tc)
	}
}

func TestUpdateProperty_Temp_ActualizaSoloPricingRules(t *testing.T) {
	prop := buildTempPropertyForUpdate(t)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	rules := []domain.PricingRule{{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 15}}
	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{PricingRules: rules})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	got := repo.savedViaTx.TempConfig().PricingRules()
	if len(got) != 1 || got[0].DiscountPct != 15 {
		t.Errorf("PricingRules: got %+v, want [{weekly 7 15}]", got)
	}
}

func TestUpdateProperty_Temp_ConfigInvalida_RetornaErrorDeDominio(t *testing.T) {
	prop := buildTempPropertyForUpdate(t)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{MinNights: intPtr(0)})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (min_nights inválido)", err)
	}
}

func TestUpdateProperty_Temp_PropiedadReservada_RetornaPreconditionFailed(t *testing.T) {
	prop := buildTempPropertyForUpdate(t)
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewUpdatePropertyUseCase(nil, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{NightPrice: floatPtr(80)})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed (propiedad reservada)", err)
	}
}

// ─── Persistencia transaccional ────────────────────────────────────────────

func TestUpdateProperty_RepoSinSoporteDeTx_RetornaError(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &repoWithoutTx{property: prop}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{})

	if err == nil {
		t.Fatal("Execute: esperaba un error porque el repo no implementa SaveWithTx")
	}
}

func TestUpdateProperty_ErrorAlIniciarTransaccion_RetornaError(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("conexión rechazada")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(boom)
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestUpdateProperty_ErrorEnSaveWithTx_HaceRollback(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo de escritura en outbox")
	repo := &fakePropertyRepo{
		findByIDFn:    func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
		saveWithTxErr: boom,
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestUpdateProperty_ErrorEnCommit_SePropagaSinEnvolver(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo al confirmar la transacción")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(boom)
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

// ─── Happy path combinado ───────────────────────────────────────────────────

func TestUpdateProperty_HappyPath_ActualizaMultiplesCamposALaVez(t *testing.T) {
	prop := buildTempPropertyForUpdate(t)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewUpdatePropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), "prop-1", application.UpdatePropertyDTO{
		Title: strPtr("Depto full renovado"), Address: strPtr("Nueva Dirección 456"),
		PetPolicy: strPtr("ALLOWED"), NightPrice: floatPtr(80),
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	saved := repo.savedViaTx
	if saved.Title() != "Depto full renovado" {
		t.Errorf("Title: got %q", saved.Title())
	}
	if saved.Location().Address() != "Nueva Dirección 456" {
		t.Errorf("Address: got %q", saved.Location().Address())
	}
	if saved.PetPolicy() != domain.PetPolicyAllowed {
		t.Errorf("PetPolicy: got %q", saved.PetPolicy())
	}
	if saved.TempConfig().NightPrice() != 80 {
		t.Errorf("NightPrice: got %v", saved.TempConfig().NightPrice())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}
