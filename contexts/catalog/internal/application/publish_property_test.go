package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

// repoWithoutTx implementa ports.PropertyRepository pero NO SaveWithTx — se usa
// para probar la rama en la que el repo configurado no soporta transacciones outbox.
type repoWithoutTx struct {
	property *domain.Property // devuelto por FindByID; nil por defecto (simula "no encontrado")
}

func (r *repoWithoutTx) Save(ctx context.Context, property *domain.Property) error { return nil }
func (r *repoWithoutTx) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	return r.property, nil
}
func (r *repoWithoutTx) FindAll(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
	return nil, 0, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// newMockDB está definido en add_property_media_test.go (mismo paquete).

func validPublishDTO() application.PublishPropertyDTO {
	return application.PublishPropertyDTO{
		ID: "prop-1", OwnerID: "owner-1", Title: "Depto en Palermo", Description: "Luminoso",
		Price: 100000, Currency: "USD", Latitude: -34.6, Longitude: -58.4, Address: "Calle Falsa 123",
		OperationType: "SALE", PetPolicy: "NOT_ALLOWED",
	}
}

// ─── Validación de dominio (falla antes de tocar la DB) ────────────────────

func TestPublishProperty_PrecioInvalido_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.Price = -100
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (precio inválido)", err)
	}
}

func TestPublishProperty_MonedaInvalida_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.Currency = "EUR"
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (moneda inválida)", err)
	}
}

func TestPublishProperty_DireccionVacia_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.Address = ""
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (dirección vacía)", err)
	}
}

func TestPublishProperty_TituloVacio_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.Title = ""
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (título vacío)", err)
	}
}

func TestPublishProperty_OperationTypeInvalido_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.OperationType = "ALQUILER_TEMPORAL_RARO"
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (operation_type inválido)", err)
	}
}

func TestPublishProperty_PetPolicyInvalida_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.PetPolicy = "SOLO_PERROS"
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (pet_policy inválida)", err)
	}
}

func TestPublishProperty_TempConfigInvalido_RetornaErrorDeDominio(t *testing.T) {
	dto := validPublishDTO()
	dto.OperationType = "TEMP"
	dto.MinNights = 0 // inválido: NewTempConfig exige >= 1
	uc := application.NewPublishPropertyUseCase(nil, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (min_nights inválido)", err)
	}
}

// ─── Defaults ───────────────────────────────────────────────────────────────

func TestPublishProperty_OperationTypeVacio_DefaultSale(t *testing.T) {
	dto := validPublishDTO()
	dto.OperationType = ""
	repo := &fakePropertyRepo{}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewPublishPropertyUseCase(db, repo)

	if err := uc.Execute(context.Background(), dto); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.OperationType() != domain.OperationSale {
		t.Errorf("OperationType default: got %q, want %q", repo.savedViaTx.OperationType(), domain.OperationSale)
	}
}

func TestPublishProperty_PetPolicyVacia_DefaultNotAllowed(t *testing.T) {
	dto := validPublishDTO()
	dto.PetPolicy = ""
	repo := &fakePropertyRepo{}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewPublishPropertyUseCase(db, repo)

	if err := uc.Execute(context.Background(), dto); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx.PetPolicy() != domain.PetPolicyNotAllowed {
		t.Errorf("PetPolicy default: got %q, want %q", repo.savedViaTx.PetPolicy(), domain.PetPolicyNotAllowed)
	}
}

// ─── Persistencia transaccional ────────────────────────────────────────────

func TestPublishProperty_RepoSinSoporteDeTx_RetornaError(t *testing.T) {
	dto := validPublishDTO()
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // el defer tx.Rollback() sí llega a ejecutarse contra el driver
	uc := application.NewPublishPropertyUseCase(db, &repoWithoutTx{})

	err := uc.Execute(context.Background(), dto)

	if err == nil {
		t.Fatal("Execute: esperaba error porque el repo no implementa SaveWithTx")
	}
}

func TestPublishProperty_ErrorAlIniciarTransaccion_RetornaError(t *testing.T) {
	dto := validPublishDTO()
	boom := errors.New("conexión rechazada")
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(boom)
	uc := application.NewPublishPropertyUseCase(db, &fakePropertyRepo{})

	err := uc.Execute(context.Background(), dto)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestPublishProperty_ErrorEnSaveWithTx_HaceRollback(t *testing.T) {
	dto := validPublishDTO()
	boom := errors.New("fallo de escritura en outbox")
	repo := &fakePropertyRepo{saveWithTxErr: boom}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()
	uc := application.NewPublishPropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), dto)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestPublishProperty_ErrorEnCommit_SePropagaSinEnvolver(t *testing.T) {
	dto := validPublishDTO()
	boom := errors.New("fallo al confirmar la transacción")
	repo := &fakePropertyRepo{}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(boom)
	uc := application.NewPublishPropertyUseCase(db, repo)

	err := uc.Execute(context.Background(), dto)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

// ─── Happy path ─────────────────────────────────────────────────────────────

func TestPublishProperty_HappyPath_Venta_GuardaConEventoPublished(t *testing.T) {
	dto := validPublishDTO()
	repo := &fakePropertyRepo{}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewPublishPropertyUseCase(db, repo)

	if err := uc.Execute(context.Background(), dto); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedViaTx == nil {
		t.Fatal("SaveWithTx: no fue invocado")
	}
	if repo.savedViaTx.ID() != "prop-1" || repo.savedViaTx.OwnerID() != "owner-1" || repo.savedViaTx.Title() != "Depto en Palermo" {
		t.Errorf("SaveWithTx property: got ID=%q OwnerID=%q Title=%q", repo.savedViaTx.ID(), repo.savedViaTx.OwnerID(), repo.savedViaTx.Title())
	}

	events := repo.savedViaTx.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos registrados: got %d, want 1", len(events))
	}
	if _, ok := events[0].(domain.PropertyPublished); !ok {
		t.Errorf("evento: got %T, want domain.PropertyPublished", events[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestPublishProperty_HappyPath_Temp_AplicaTempConfigYSnapshotDelEvento(t *testing.T) {
	dto := validPublishDTO()
	dto.OperationType = "TEMP"
	dto.MinNights = 2
	dto.MaxNights = 30
	dto.NightPrice = 50
	dto.CleaningFee = 20
	dto.SecurityDeposit = 100
	dto.CheckInTime = "15:00"
	dto.CheckOutTime = "11:00"

	repo := &fakePropertyRepo{}
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()
	uc := application.NewPublishPropertyUseCase(db, repo)

	if err := uc.Execute(context.Background(), dto); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	tc := repo.savedViaTx.TempConfig()
	if tc.NightPrice() != 50 || tc.CleaningFee() != 20 || tc.SecurityDeposit() != 100 ||
		tc.CheckInTime() != "15:00" || tc.CheckOutTime() != "11:00" {
		t.Errorf("TempConfig: got %+v", tc)
	}

	events := repo.savedViaTx.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos registrados: got %d, want 1", len(events))
	}
	evt, ok := events[0].(domain.PropertyPublished)
	if !ok {
		t.Fatalf("evento: got %T, want domain.PropertyPublished", events[0])
	}
	// El comentario del código es explícito: el evento se registra DESPUÉS de
	// SetTempConfig justamente para que el snapshot lleve los valores reales.
	if evt.Snapshot.NightPrice != 50 || evt.Snapshot.CleaningFee != 20 {
		t.Errorf("Snapshot del evento: got %+v, want reflejar el TempConfig aplicado", evt.Snapshot)
	}
}
