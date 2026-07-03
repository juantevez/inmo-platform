package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

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

func activatedContractFixture(t *testing.T) *domain.Contract {
	t.Helper()
	c := draftContractFixture(t)
	if err := c.Activate(); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	return c
}

// ─── Save ───────────────────────────────────────────────────────────────────

func TestContractRepository_Save_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))
	repo := postgres.NewContractRepository(db)

	err := repo.Save(context.Background(), draftContractFixture(t))
	assertInternalError(t, err)
}

func TestContractRepository_Save_ErrorEnSaveWithTx_HaceRollback(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback() // Commit() nunca se alcanza, así que el rollback diferido se ejecuta de verdad.
	repo := postgres.NewContractRepository(db)

	err := repo.Save(context.Background(), draftContractFixture(t))
	assertInternalError(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractRepository_Save_Exitoso_Commitea(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := postgres.NewContractRepository(db)

	if err := repo.Save(context.Background(), draftContractFixture(t)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ─── SaveWithTx ─────────────────────────────────────────────────────────────

func TestContractRepository_SaveWithTx_ErrorAlInsertarElContrato(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()
	repo := postgres.NewContractRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	err = repo.SaveWithTx(context.Background(), tx, draftContractFixture(t))
	assertInternalError(t, err)
}

func TestContractRepository_SaveWithTx_SinEventos_NoInsertaEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnResult(sqlmock.NewResult(0, 1))
	// Ninguna expectativa de INSERT INTO contracts_outbox_events: un contrato
	// recién creado (sin Activate()) no tiene eventos de dominio pendientes.
	repo := postgres.NewContractRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	if err := repo.SaveWithTx(context.Background(), tx, draftContractFixture(t)); err != nil {
		t.Fatalf("SaveWithTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractRepository_SaveWithTx_ConEventos_LosInsertaEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contracts_outbox_events`).
		WithArgs(sqlmock.AnyArg(), "Contract", "contract-1", "contracts.contract.activated", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := postgres.NewContractRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	if err := repo.SaveWithTx(context.Background(), tx, activatedContractFixture(t)); err != nil {
		t.Fatalf("SaveWithTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestContractRepository_SaveWithTx_ErrorAlInsertarEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO contracts`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contracts_outbox_events`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()
	repo := postgres.NewContractRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	err = repo.SaveWithTx(context.Background(), tx, activatedContractFixture(t))
	assertInternalError(t, err)
}

// ─── FindByID ───────────────────────────────────────────────────────────────

var contractColumns = []string{
	"property_id", "tenant_id", "owner_id", "rent_amount", "currency",
	"start_date", "end_date", "adjustment_index", "adjustment_period_months", "state", "created_at",
}

func TestContractRepository_FindByID_NoEncontrado_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT property_id, tenant_id, owner_id, rent_amount, currency,`).
		WithArgs("contract-x").
		WillReturnRows(sqlmock.NewRows(contractColumns))
	repo := postgres.NewContractRepository(db)

	c, err := repo.FindByID(context.Background(), "contract-x")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if c != nil {
		t.Fatalf("esperaba nil, obtuve %+v", c)
	}
}

func TestContractRepository_FindByID_ErrorDeScan(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT property_id, tenant_id, owner_id, rent_amount, currency,`).
		WithArgs("contract-1").
		WillReturnError(errors.New("timeout de red"))
	repo := postgres.NewContractRepository(db)

	_, err := repo.FindByID(context.Background(), "contract-1")
	assertInternalError(t, err)
}

func TestContractRepository_FindByID_MontoCorrompido_RetornaErrorInterno(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT property_id, tenant_id, owner_id, rent_amount, currency,`).
		WithArgs("contract-1").
		WillReturnRows(sqlmock.NewRows(contractColumns).
			AddRow("prop-1", "tenant-1", "owner-1", 0.0, "ARS", now, now.Add(24*time.Hour), "ICL", 4, "DRAFT", now))
	repo := postgres.NewContractRepository(db)

	_, err := repo.FindByID(context.Background(), "contract-1")
	assertInternalError(t, err)
}

func TestContractRepository_FindByID_FechasCorrompidas_RetornaErrorInterno(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT property_id, tenant_id, owner_id, rent_amount, currency,`).
		WithArgs("contract-1").
		WillReturnRows(sqlmock.NewRows(contractColumns).
			// end_date anterior a start_date: NewTimeline la rechaza.
			AddRow("prop-1", "tenant-1", "owner-1", 100000.0, "ARS", now, now.Add(-24*time.Hour), "ICL", 4, "DRAFT", now))
	repo := postgres.NewContractRepository(db)

	_, err := repo.FindByID(context.Background(), "contract-1")
	assertInternalError(t, err)
}

func TestContractRepository_FindByID_Exitoso_MapeaTodosLosCampos(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	start := now
	end := now.Add(365 * 24 * time.Hour)
	mock.ExpectQuery(`SELECT property_id, tenant_id, owner_id, rent_amount, currency,`).
		WithArgs("contract-1").
		WillReturnRows(sqlmock.NewRows(contractColumns).
			AddRow("prop-1", "tenant-1", "owner-1", 150000.0, "USD", start, end, "IPC", 6, "ACTIVE", now))
	repo := postgres.NewContractRepository(db)

	c, err := repo.FindByID(context.Background(), "contract-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if c == nil {
		t.Fatal("esperaba un contrato, obtuve nil")
	}
	if c.ID() != "contract-1" || c.PropertyID() != "prop-1" || c.TenantID() != "tenant-1" || c.OwnerID() != "owner-1" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", c)
	}
	if c.RentAmount().Amount() != 150000.0 || c.RentAmount().Currency() != "USD" {
		t.Fatalf("rent amount mapeado incorrectamente: %+v", c.RentAmount())
	}
	if !c.Timeline().StartDate().Equal(start) || !c.Timeline().EndDate().Equal(end) {
		t.Fatalf("timeline mapeado incorrectamente: %+v", c.Timeline())
	}
	if c.Index() != domain.IndexIPC || c.AdjustmentPeriod() != domain.AdjustmentPeriod(6) {
		t.Fatalf("index/period mapeados incorrectamente: %s / %d", c.Index(), c.AdjustmentPeriod())
	}
	if c.State() != domain.StateActive {
		t.Fatalf("state: got %s, want %s", c.State(), domain.StateActive)
	}
	if !c.CreatedAt().Equal(now) {
		t.Fatalf("createdAt: got %v, want %v", c.CreatedAt(), now)
	}
}

// ─── FindAllActive ──────────────────────────────────────────────────────────

func TestContractRepository_FindAllActive_EsUnEsqueletoSinImplementar(t *testing.T) {
	// FindAllActive todavía no está implementado (ver comentario en el código
	// fuente): siempre retorna nil, nil sin tocar la DB. Documentamos ese
	// comportamiento actual, no necesariamente el deseado a futuro.
	db, _ := newMockDB(t)
	repo := postgres.NewContractRepository(db)

	contracts, err := repo.FindAllActive(context.Background())
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if contracts != nil {
		t.Fatalf("esperaba nil, obtuve %+v", contracts)
	}
}
