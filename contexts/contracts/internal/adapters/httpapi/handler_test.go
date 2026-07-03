package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

var errFixtureNoConfigurada = errors.New("fixture no configurada")

// fakeContractRepo implementa ports.ContractRepository + SaveWithTx, para poder
// ejercitar tanto CreateContractUseCase como ActivateContractUseCase (que exige
// vía type assertion un repositorio con soporte transaccional).
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

// fakeContractRepoNoTx implementa ports.ContractRepository SIN SaveWithTx, para
// probar la rama en la que ActivateContractUseCase detecta que el repositorio
// configurado no soporta transacciones outbox.
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

func validCreateContractBody() []byte {
	body, _ := json.Marshal(map[string]any{
		"id":                "contract-1",
		"property_id":       "prop-1",
		"tenant_id":         "tenant-1",
		"owner_id":          "owner-1",
		"amount":            100000,
		"currency":          "ARS",
		"start_date":        time.Now().Format(time.RFC3339),
		"end_date":          time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339),
		"adjustment_index":  "ICL",
		"adjustment_period": 4,
	})
	return body
}

func decodeAppError(t *testing.T, rec *httptest.ResponseRecorder) apperr.AppError {
	t.Helper()
	var ae apperr.AppError
	if err := json.Unmarshal(rec.Body.Bytes(), &ae); err != nil {
		t.Fatalf("no se pudo decodificar AppError: %v, body=%s", err, rec.Body.String())
	}
	return ae
}

// ─── ContractHandler.Create ─────────────────────────────────────────────────

func TestContractHandler_Create_JSONMalformado_Retorna400(t *testing.T) {
	repo := &fakeContractRepo{}
	h := httpapi.NewContractHandler(application.NewCreateContractUseCase(repo), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", bytes.NewBufferString("{invalido"))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestContractHandler_Create_Exitoso_Retorna201(t *testing.T) {
	var saved *domain.Contract
	repo := &fakeContractRepo{
		saveFn: func(ctx context.Context, c *domain.Contract) error {
			saved = c
			return nil
		},
	}
	h := httpapi.NewContractHandler(application.NewCreateContractUseCase(repo), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", bytes.NewReader(validCreateContractBody()))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if saved == nil || saved.ID() != "contract-1" {
		t.Fatalf("el contrato no se guardó correctamente: %+v", saved)
	}
}

func TestContractHandler_Create_ErrorDeValidacionDelDominio_Retorna400(t *testing.T) {
	repo := &fakeContractRepo{}
	h := httpapi.NewContractHandler(application.NewCreateContractUseCase(repo), nil)

	body, _ := json.Marshal(map[string]any{
		"id": "contract-1", "property_id": "prop-1", "tenant_id": "tenant-1", "owner_id": "owner-1",
		"amount": 0, "currency": "ARS", // monto inválido -> NewRentAmount falla
		"start_date": time.Now().Format(time.RFC3339), "end_date": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if ae := decodeAppError(t, rec); ae.Type != apperr.TypeBadRequest {
		t.Fatalf("error type: got %s", ae.Type)
	}
}

func TestContractHandler_Create_ErrorNoTipadoDelRepo_Retorna500(t *testing.T) {
	repo := &fakeContractRepo{
		saveFn: func(ctx context.Context, c *domain.Contract) error {
			return errors.New("fallo de conexión crudo")
		},
	}
	h := httpapi.NewContractHandler(application.NewCreateContractUseCase(repo), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts", bytes.NewReader(validCreateContractBody()))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// ─── ContractHandler.Activate ───────────────────────────────────────────────

func draftContract(t *testing.T) *domain.Contract {
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

func TestContractHandler_Activate_SinIDEnQuery_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	repo := &fakeContractRepo{}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestContractHandler_Activate_Exitoso_Retorna200(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) {
			return draftContract(t), nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error { return nil },
	}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=contract-1", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractHandler_Activate_ContratoNoEncontrado_Retorna404(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza en esta rama, así que el rollback diferido se ejecuta de verdad.
	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return nil, nil },
	}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=no-existe", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractHandler_Activate_TransicionInvalida_Retorna412(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza en esta rama, así que el rollback diferido se ejecuta de verdad.
	rent, _ := domain.NewRentAmount(100000, "ARS")
	tl, _ := domain.NewTimeline(time.Now(), time.Now().Add(24*time.Hour))
	alreadyActive := domain.RehydrateContract("contract-1", "prop-1", "tenant-1", "owner-1", rent, tl, domain.IndexICL, domain.AdjustmentPeriod(4), domain.StateActive, time.Now())

	repo := &fakeContractRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return alreadyActive, nil },
	}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=contract-1", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractHandler_Activate_RepoSinSoporteTransaccional_Retorna500(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza en esta rama, así que el rollback diferido se ejecuta de verdad.

	repo := &fakeContractRepoNoTx{
		findByIDFn: func(ctx context.Context, id string) (*domain.Contract, error) { return draftContract(t), nil },
	}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=contract-1", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractHandler_Activate_ErrorAlIniciarTransaccion_Retorna500(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	repo := &fakeContractRepo{}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=contract-1", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestContractHandler_Activate_ErrorAlConfirmarTransaccion_Retorna500(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))

	repo := &fakeContractRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Contract, error) { return draftContract(t), nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, c *domain.Contract) error { return nil },
	}
	h := httpapi.NewContractHandler(nil, application.NewActivateContractUseCase(db, repo))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contracts/activate?id=contract-1", nil)
	rec := httptest.NewRecorder()
	h.Activate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
