package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/crm/internal/adapters/postgres"
	"inmo.platform/contexts/crm/internal/domain"
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

func assertInternalError(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func leadFixture(t *testing.T) *domain.Lead {
	t.Helper()
	l, err := domain.NewLead("lead-1", "prop-1", "Juan Pérez", "juan@test.com", "")
	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	return l
}

var leadColumns = []string{
	"id", "property_id", "client_name", "email", "phone", "state", "visit_scheduled_at", "created_at", "updated_at",
}

// ─── Save ───────────────────────────────────────────────────────────────────

func TestPostgresLeadRepository_Save_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))
	repo := postgres.NewPostgresLeadRepository(db)

	err := repo.Save(context.Background(), leadFixture(t), "", nil)
	assertInternalError(t, err)
}

func TestPostgresLeadRepository_Save_ErrorAlPersistirElLead(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()
	repo := postgres.NewPostgresLeadRepository(db)

	err := repo.Save(context.Background(), leadFixture(t), "", nil)
	assertInternalError(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPostgresLeadRepository_Save_SinEvento_NoInsertaEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).WillReturnResult(sqlmock.NewResult(0, 1))
	// Ninguna expectativa de INSERT INTO crm_outbox_events: eventName vacío no
	// debería tocar la tabla outbox.
	mock.ExpectCommit()
	repo := postgres.NewPostgresLeadRepository(db)

	if err := repo.Save(context.Background(), leadFixture(t), "", nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPostgresLeadRepository_Save_ConEvento_LoInsertaEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO crm_outbox_events`).
		WithArgs("crm.lead.visit_scheduled", []byte(`{}`), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := postgres.NewPostgresLeadRepository(db)

	if err := repo.Save(context.Background(), leadFixture(t), "crm.lead.visit_scheduled", []byte(`{}`)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPostgresLeadRepository_Save_ConVisitaAgendada_PersisteLaFecha(t *testing.T) {
	db, mock := newMockDB(t)
	visitAt := time.Now().Add(48 * time.Hour)
	l := leadFixture(t)
	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted setup: %v", err)
	}
	if err := l.ScheduleVisit(visitAt); err != nil {
		t.Fatalf("ScheduleVisit setup: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).
		WithArgs("lead-1", "prop-1", "Juan Pérez", "juan@test.com", nil, "VISIT_SCHEDULED", visitAt, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := postgres.NewPostgresLeadRepository(db)

	if err := repo.Save(context.Background(), l, "", nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPostgresLeadRepository_Save_ErrorAlInsertarEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO crm_outbox_events`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()
	repo := postgres.NewPostgresLeadRepository(db)

	err := repo.Save(context.Background(), leadFixture(t), "crm.lead.visit_scheduled", []byte(`{}`))
	assertInternalError(t, err)
}

func TestPostgresLeadRepository_Save_ErrorAlConfirmarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO leads`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))
	repo := postgres.NewPostgresLeadRepository(db)

	err := repo.Save(context.Background(), leadFixture(t), "", nil)
	assertInternalError(t, err)
}

// ─── GetByID ────────────────────────────────────────────────────────────────

func TestPostgresLeadRepository_GetByID_NoEncontrado_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at,`).
		WithArgs("lead-x").
		WillReturnRows(sqlmock.NewRows(leadColumns))
	repo := postgres.NewPostgresLeadRepository(db)

	l, err := repo.GetByID(context.Background(), "lead-x")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if l != nil {
		t.Fatalf("esperaba nil, obtuve %+v", l)
	}
}

func TestPostgresLeadRepository_GetByID_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at,`).
		WithArgs("lead-1").
		WillReturnError(errors.New("timeout de red"))
	repo := postgres.NewPostgresLeadRepository(db)

	_, err := repo.GetByID(context.Background(), "lead-1")
	assertInternalError(t, err)
}

func TestPostgresLeadRepository_GetByID_SinVisitaAgendada_MapeaPunteroNil(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at,`).
		WithArgs("lead-1").
		WillReturnRows(sqlmock.NewRows(leadColumns).
			AddRow("lead-1", "prop-1", "Juan Pérez", "juan@test.com", nil, "NEW", nil, now, now))
	repo := postgres.NewPostgresLeadRepository(db)

	l, err := repo.GetByID(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if l.VisitScheduledAt != nil {
		t.Fatalf("esperaba VisitScheduledAt nil, obtuve %v", l.VisitScheduledAt)
	}
	if l.Phone != "" {
		t.Fatalf("phone: got %q, want vacío", l.Phone)
	}
	if l.ID != "lead-1" || l.State != domain.StateNew {
		t.Fatalf("mapeo incorrecto: %+v", l)
	}
}

func TestPostgresLeadRepository_GetByID_ConVisitaAgendada_MapeaLaFecha(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	visitAt := now.Add(48 * time.Hour)
	mock.ExpectQuery(`SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at,`).
		WithArgs("lead-1").
		WillReturnRows(sqlmock.NewRows(leadColumns).
			AddRow("lead-1", "prop-1", "Juan Pérez", "juan@test.com", "+54911234567", "VISIT_SCHEDULED", visitAt, now, now))
	repo := postgres.NewPostgresLeadRepository(db)

	l, err := repo.GetByID(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if l.VisitScheduledAt == nil || !l.VisitScheduledAt.Equal(visitAt) {
		t.Fatalf("visit_scheduled_at mapeado incorrectamente: %v", l.VisitScheduledAt)
	}
	if l.Phone != "+54911234567" || l.State != domain.StateVisitScheduled {
		t.Fatalf("mapeo incorrecto: %+v", l)
	}
}

func TestPostgresLeadRepository_GetByID_ErrorDeScan(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at,`).
		WithArgs("lead-1").
		WillReturnRows(sqlmock.NewRows(leadColumns).
			AddRow(nil, "prop-1", "Juan Pérez", "juan@test.com", nil, "NEW", nil, time.Now(), time.Now()))
	repo := postgres.NewPostgresLeadRepository(db)

	_, err := repo.GetByID(context.Background(), "lead-1")
	assertInternalError(t, err)
}
