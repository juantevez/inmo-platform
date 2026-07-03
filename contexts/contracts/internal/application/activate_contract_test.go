package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

var errFixtureNoConfigurada = errors.New("fixture no configurada")

// fakeContractRepo implementa ports.ContractRepository + SaveWithTx, para
// poder ejercitar tanto el camino feliz como la rama de "repo sin soporte
// transaccional" del use case (que exige SaveWithTx vía type assertion).
type fakeContractRepo struct {
	saveFn          func(ctx context.Context, c *domain.Contract) error
	findByIDFn      func(ctx context.Context, id string) (*domain.Contract, error)
	findAllActiveFn func(ctx context.Context) ([]*domain.Contract, error)
	saveWithTxFn    func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error
}

func (f *fakeContractRepo) Save(ctx context.Context, c *domain.Contract) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, c)
	}
	return errFixtureNoConfigurada
}

func (f *fakeContractRepo) FindByID(ctx context.Context, id string) (*domain.Contract, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errFixtureNoConfigurada
}

func (f *fakeContractRepo) FindAllActive(ctx context.Context) ([]*domain.Contract, error) {
	if f.findAllActiveFn != nil {
		return f.findAllActiveFn(ctx)
	}
	return nil, errFixtureNoConfigurada
}

func (f *fakeContractRepo) SaveWithTx(ctx context.Context, tx *sql.Tx, c *domain.Contract) error {
	if f.saveWithTxFn != nil {
		return f.saveWithTxFn(ctx, tx, c)
	}
	return errFixtureNoConfigurada
}

// fakeContractRepoNoTx implementa ports.ContractRepository SIN SaveWithTx,
// para probar la rama donde el use case detecta que el repo configurado no
// soporta transacciones outbox.
type fakeContractRepoNoTx struct {
	findByIDFn func(ctx context.Context, id string) (*domain.Contract, error)
}

func (f *fakeContractRepoNoTx) Save(ctx context.Context, c *domain.Contract) error { return nil }
func (f *fakeContractRepoNoTx) FindByID(ctx context.Context, id string) (*domain.Contract, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeContractRepoNoTx) FindAllActive(ctx context.Context) ([]*domain.Contract, error) {
	return nil, errFixtureNoConfigurada
}

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func assertInternalError(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func draftContractFixture(t *testing.T) *domain.Contract {
	t.Helper()
	rent, err := domain.NewRentAmount(100000, "ARS")
	if err != nil {
		t.Fatalf("NewRentAmount: %v", err)
	}
	tl, err := domain.NewTimeline(time.Now(), time.Now().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("NewTimeline: %v", err)
	}
	return domain.NewContract("contract-1", "prop-1", "tenant-1", "owner-1", rent, tl, domain.IndexICL, domain.AdjustmentPeriod(4))
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestActivateContractUseCase_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))
	uc := application.NewActivateContractUseCase(db, &fakeContractRepo{})

	err := uc.Execute(context.Background(), "contract-1")
	assertInternalError(t, err)
}

func TestActivateContractUseCase_ErrorAlBuscarElContrato_NoEsAppError(t *testing.T) {
	// El código envuelve el error de FindByID con fmt.Errorf (no apperr) —
	// se propaga tal cual, sin tipar, y el handler HTTP lo mapea a 500 por
	// default. Confirmamos ese wrapping puntual con errors.Is.
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza en esta rama.

	dbErr := errors.New("timeout de base de datos")
	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return nil, dbErr },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error envolviera %v, got %v", dbErr, err)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		t.Fatalf("no esperaba un AppError para este camino, got %+v", appErr)
	}
}

func TestActivateContractUseCase_ContratoNoEncontrado_RetornaNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return nil, nil },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-x")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("got %v, want AppError NotFound", err)
	}
}

func TestActivateContractUseCase_TransicionInvalida_RetornaPreconditionFailed(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	rent, _ := domain.NewRentAmount(100000, "ARS")
	tl, _ := domain.NewTimeline(time.Now(), time.Now().Add(24*time.Hour))
	alreadyActive := domain.RehydrateContract("contract-1", "prop-1", "tenant-1", "owner-1", rent, tl, domain.IndexICL, domain.AdjustmentPeriod(4), domain.StateActive, time.Now())

	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return alreadyActive, nil },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
	if alreadyActive.State() != domain.StateActive {
		t.Fatalf("el estado no debería haber cambiado tras el error: got %s", alreadyActive.State())
	}
}

func TestActivateContractUseCase_RepoSinSoporteTransaccional_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	repo := &fakeContractRepoNoTx{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return draftContractFixture(t), nil },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-1")
	assertInternalError(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestActivateContractUseCase_ErrorEnSaveWithTx_SePropagaTalCual(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	sentinel := apperr.NewInternal("fallo simulado de guardado", nil)
	repo := &fakeContractRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Contract, error) { return draftContractFixture(t), nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error { return sentinel },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("esperaba que el error de SaveWithTx se propague tal cual: got %v", err)
	}
}

func TestActivateContractUseCase_ErrorAlConfirmarTransaccion_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))

	repo := &fakeContractRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Contract, error) { return draftContractFixture(t), nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error { return nil },
	}
	uc := application.NewActivateContractUseCase(db, repo)

	err := uc.Execute(context.Background(), "contract-1")
	assertInternalError(t, err)
}

func TestActivateContractUseCase_Exitoso_ActivaYCommitea(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	var savedContract *domain.Contract
	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return draftContractFixture(t), nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error {
			savedContract = c
			return nil
		},
	}
	uc := application.NewActivateContractUseCase(db, repo)

	if err := uc.Execute(context.Background(), "contract-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if savedContract == nil || savedContract.State() != domain.StateActive {
		t.Fatalf("el contrato guardado debería estar ACTIVE: %+v", savedContract)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
