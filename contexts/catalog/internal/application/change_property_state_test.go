package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

// ─── Fakes compartidos por los tests de application (catalog) ─────────────────

type fakePropertyRepo struct {
	findByIDFn    func(ctx context.Context, id string) (*domain.Property, error)
	saveFn        func(ctx context.Context, property *domain.Property) error
	findAllFn     func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error)
	saveWithTxErr error

	saveCalled    bool
	savedProperty *domain.Property
	savedViaTx    *domain.Property
}

// SaveWithTx satisface la interfaz privada TxRepository que exigen Publish/Update
// (type-assertion interna sobre ports.PropertyRepository).
func (f *fakePropertyRepo) SaveWithTx(ctx context.Context, tx *sql.Tx, property *domain.Property) error {
	if f.saveWithTxErr != nil {
		return f.saveWithTxErr
	}
	f.savedViaTx = property
	return nil
}

func (f *fakePropertyRepo) Save(ctx context.Context, property *domain.Property) error {
	f.saveCalled = true
	f.savedProperty = property
	if f.saveFn != nil {
		return f.saveFn(ctx, property)
	}
	return nil
}
func (f *fakePropertyRepo) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, nil
}
func (f *fakePropertyRepo) FindAll(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
	if f.findAllFn != nil {
		return f.findAllFn(ctx, filters)
	}
	return nil, 0, errors.New("FindAll: fixture no configurada")
}

type fakeEventPublisher struct {
	published []ddd.DomainEvent
	err       error
}

func (f *fakeEventPublisher) Publish(ctx context.Context, events ...ddd.DomainEvent) error {
	f.published = append(f.published, events...)
	return f.err
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func buildProperty(t *testing.T, id, ownerID string) *domain.Property {
	t.Helper()
	price, err := domain.NewPrice(1000, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	loc, err := domain.NewLocation(-34.6, -58.4, "Calle Falsa 123")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	p, err := domain.NewProperty(id, ownerID, "Depto", "desc", price, loc, domain.OperationSale, domain.PetPolicyNotAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	return p
}

func newChangeStateUseCase(repo ports.PropertyRepository, publisher ports.EventPublisher) *application.ChangePropertyStateUseCase {
	return application.NewChangePropertyStateUseCase(repo, publisher)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestChangeState_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ActionReserve)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestChangeState_PropiedadNoExiste_RetornaErrNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "no-existe", application.ActionReserve)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestChangeState_AccionNoReconocida_RetornaErrBadRequest(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ChangeStateAction("VOLAR"))

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
	if repo.saveCalled {
		t.Error("Save: no debería invocarse ante una acción no reconocida")
	}
}

func TestChangeState_Reserve_HappyPath_GuardaYPublicaEvento(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	publisher := &fakeEventPublisher{}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionReserve)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if !repo.saveCalled || repo.savedProperty.State() != domain.StateReserved {
		t.Errorf("Save: got called=%v state=%v, want called=true state=RESERVED", repo.saveCalled, repo.savedProperty.State())
	}
	if len(publisher.published) != 1 {
		t.Fatalf("Publish: got %d eventos, want 1", len(publisher.published))
	}
	evt, ok := publisher.published[0].(domain.PropertyStateChanged)
	if !ok {
		t.Fatalf("Publish: got %T, want domain.PropertyStateChanged", publisher.published[0])
	}
	if evt.OldState != domain.StateAvailable || evt.NewState != domain.StateReserved {
		t.Errorf("evento: got OldState=%q NewState=%q, want AVAILABLE->RESERVED", evt.OldState, evt.NewState)
	}
}

func TestChangeState_Reserve_PropiedadYaReservada_RetornaErrorYNoGuarda(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	if err := prop.Reserve(); err != nil {
		t.Fatalf("setup Reserve: %v", err)
	}
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	publisher := &fakeEventPublisher{}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionReserve)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed", err)
	}
	if repo.saveCalled {
		t.Error("Save: no debería invocarse si la transición de estado falló")
	}
	if len(publisher.published) != 0 {
		t.Error("Publish: no debería invocarse si la transición de estado falló")
	}
}

func TestChangeState_Close_HappyPath(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ActionClose)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedProperty.State() != domain.StateClosed {
		t.Errorf("State: got %q, want CLOSED", repo.savedProperty.State())
	}
}

func TestChangeState_Close_YaCerrada_RetornaError(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	if err := prop.Close(); err != nil {
		t.Fatalf("setup Close: %v", err)
	}
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ActionClose)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed", err)
	}
}

func TestChangeState_PutUnderRepair_HappyPath(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	publisher := &fakeEventPublisher{}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionUnderRepair)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedProperty.State() != domain.StateUnderRepair {
		t.Errorf("State: got %q, want UNDER_REPAIR", repo.savedProperty.State())
	}
	if len(publisher.published) != 1 {
		t.Errorf("Publish: got %d eventos, want 1", len(publisher.published))
	}
}

func TestChangeState_PutUnderRepair_Idempotente_GuardaPeroNoPublica(t *testing.T) {
	// domain.Property.PutUnderRepair() es idempotente: si ya está en reparación,
	// retorna nil SIN registrar un nuevo evento. El use case igual llama a Save()
	// (porque err == nil), pero PullEvents() devuelve vacío, así que Publish no se invoca.
	prop := buildProperty(t, "prop-1", "owner-1")
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("setup PutUnderRepair: %v", err)
	}
	prop.PullEvents() // drena el evento de la transición de setup (AVAILABLE→UNDER_REPAIR)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	publisher := &fakeEventPublisher{}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionUnderRepair)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if !repo.saveCalled {
		t.Error("Save: debería invocarse igual aunque la transición sea un no-op")
	}
	if len(publisher.published) != 0 {
		t.Errorf("Publish: got %d eventos, want 0 (PutUnderRepair idempotente no genera evento)", len(publisher.published))
	}
}

func TestChangeState_ReleaseRepair_HappyPath(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	if err := prop.PutUnderRepair(); err != nil {
		t.Fatalf("setup PutUnderRepair: %v", err)
	}
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ActionReleaseRepair)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.savedProperty.State() != domain.StateAvailable {
		t.Errorf("State: got %q, want AVAILABLE", repo.savedProperty.State())
	}
}

func TestChangeState_ReleaseRepair_NoEstaEnReparacion_RetornaError(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1") // AVAILABLE, nunca entró a reparación
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newChangeStateUseCase(repo, &fakeEventPublisher{})

	err := uc.Execute(context.Background(), "prop-1", application.ActionReleaseRepair)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed", err)
	}
}

func TestChangeState_ErrorAlGuardar_RetornaErrorSinPublicar(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo de escritura")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
		saveFn:     func(ctx context.Context, property *domain.Property) error { return boom },
	}
	publisher := &fakeEventPublisher{}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionReserve)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
	if len(publisher.published) != 0 {
		t.Error("Publish: no debería invocarse si Save falló")
	}
}

func TestChangeState_ErrorAlPublicar_NoFallaLaOperacion(t *testing.T) {
	// El use case ignora deliberadamente el error de Publish ("_ = uc.publisher.Publish(...)").
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	publisher := &fakeEventPublisher{err: errors.New("nats no disponible")}
	uc := newChangeStateUseCase(repo, publisher)

	err := uc.Execute(context.Background(), "prop-1", application.ActionReserve)

	if err != nil {
		t.Fatalf("Execute: got %v, want nil (el error de Publish no debe propagarse)", err)
	}
}
