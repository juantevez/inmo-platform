package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

var reservationColumns = []string{
	"id", "property_id", "tenant_id", "owner_id",
	"check_in_date", "check_out_date", "nights",
	"night_price_snapshot", "discount_pct", "cleaning_fee", "security_deposit", "total_amount",
	"status", "guest_message", "confirmed_at", "cancelled_at", "created_at", "updated_at",
}

func newReservationFixture(t *testing.T) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 3, 10000, 0, 1500, 5000, 31500, "Llegamos tarde")
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}
	return r // trae 1 evento pendiente: ReservationCreatedEvent
}

func reservationRow(id string, confirmedAt, cancelledAt *time.Time) *sqlmock.Rows {
	now := time.Now()
	checkIn := now.Add(24 * time.Hour)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	var confVal, cancVal interface{}
	if confirmedAt != nil {
		confVal = *confirmedAt
	}
	if cancelledAt != nil {
		cancVal = *cancelledAt
	}
	return sqlmock.NewRows(reservationColumns).AddRow(
		id, "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 3,
		10000.0, 0.0, 1500.0, 5000.0, 31500.0,
		"PENDING_APPROVAL", "Llegamos tarde", confVal, cancVal, now, now,
	)
}

// ─── Save ───────────────────────────────────────────────────────────────────

func TestReservationRepository_Save_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))
	repo := postgres.NewReservationRepository(db)

	err := repo.Save(context.Background(), newReservationFixture(t))
	assertInternalError(t, err)
}

func TestReservationRepository_Save_ErrorEnSaveWithTx_HaceRollback(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reservations`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()
	repo := postgres.NewReservationRepository(db)

	err := repo.Save(context.Background(), newReservationFixture(t))
	assertInternalError(t, err)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_Save_Exitoso_Commitea(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reservations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contracts_outbox_events`).
		WithArgs(sqlmock.AnyArg(), "Reservation", "res-1", "contracts.reservation.created", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := postgres.NewReservationRepository(db)

	if err := repo.Save(context.Background(), newReservationFixture(t)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_Save_SinGuestMessage_GuardaComoNullNoVacio(t *testing.T) {
	// Cubre la rama vacía de nullStr(): un guest_message "" debe persistirse
	// como sql.NullString{Valid:false} (SQL NULL), no como string vacío.
	db, mock := newMockDB(t)
	checkIn := time.Now().Add(24 * time.Hour)
	r, err := domain.NewReservation("res-2", "prop-1", "tenant-1", "owner-1",
		checkIn, checkIn.Add(3*24*time.Hour), 3, 10000, 0, 1500, 5000, 31500, "")
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reservations`).
		WithArgs("res-2", "prop-1", "tenant-1", "owner-1",
			sqlmock.AnyArg(), sqlmock.AnyArg(), 3,
			10000.0, 0.0, 1500.0, 5000.0, 31500.0,
			"PENDING_APPROVAL", nil, nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contracts_outbox_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := postgres.NewReservationRepository(db)

	if err := repo.Save(context.Background(), r); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ─── SaveWithTx ─────────────────────────────────────────────────────────────

func TestReservationRepository_SaveWithTx_SinEventos_NoInsertaEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reservations`).WillReturnResult(sqlmock.NewResult(0, 1))
	repo := postgres.NewReservationRepository(db)

	r := newReservationFixture(t)
	r.PullEvents() // drenar el evento de creación: no debería tocar la tabla outbox

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	if err := repo.SaveWithTx(context.Background(), tx, r); err != nil {
		t.Fatalf("SaveWithTx: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_SaveWithTx_ErrorAlInsertarEnElOutbox(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reservations`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contracts_outbox_events`).WillReturnError(errors.New("fallo de escritura"))
	repo := postgres.NewReservationRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	defer tx.Rollback()

	err = repo.SaveWithTx(context.Background(), tx, newReservationFixture(t))
	assertInternalError(t, err)
}

// ─── FindByID ───────────────────────────────────────────────────────────────

func TestReservationRepository_FindByID_NoEncontrada_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("res-x").
		WillReturnRows(sqlmock.NewRows(reservationColumns))
	repo := postgres.NewReservationRepository(db)

	r, err := repo.FindByID(context.Background(), "res-x")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if r != nil {
		t.Fatalf("esperaba nil, obtuve %+v", r)
	}
}

func TestReservationRepository_FindByID_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("res-1").
		WillReturnError(errors.New("timeout de red"))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindByID(context.Background(), "res-1")
	assertInternalError(t, err)
}

func TestReservationRepository_FindByID_SinConfirmarNiCancelar_MapeaPunterosNil(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("res-1").
		WillReturnRows(reservationRow("res-1", nil, nil))
	repo := postgres.NewReservationRepository(db)

	r, err := repo.FindByID(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if r == nil {
		t.Fatal("esperaba una reserva, obtuve nil")
	}
	if r.ConfirmedAt() != nil || r.CancelledAt() != nil {
		t.Fatalf("confirmedAt/cancelledAt deberían ser nil: %+v / %+v", r.ConfirmedAt(), r.CancelledAt())
	}
	if r.ID() != "res-1" || r.GuestMessage() != "Llegamos tarde" {
		t.Fatalf("mapeo incorrecto: %+v", r)
	}
}

func TestReservationRepository_FindByID_Confirmada_MapeaConfirmedAt(t *testing.T) {
	db, mock := newMockDB(t)
	confirmedAt := time.Now().Add(-2 * time.Hour)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("res-1").
		WillReturnRows(reservationRow("res-1", &confirmedAt, nil))
	repo := postgres.NewReservationRepository(db)

	r, err := repo.FindByID(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if r.ConfirmedAt() == nil || !r.ConfirmedAt().Equal(confirmedAt) {
		t.Fatalf("confirmedAt mapeado incorrectamente: %v", r.ConfirmedAt())
	}
}

func TestReservationRepository_FindByID_Cancelada_MapeaCancelledAt(t *testing.T) {
	db, mock := newMockDB(t)
	cancelledAt := time.Now().Add(-1 * time.Hour)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("res-1").
		WillReturnRows(reservationRow("res-1", nil, &cancelledAt))
	repo := postgres.NewReservationRepository(db)

	r, err := repo.FindByID(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if r.CancelledAt() == nil || !r.CancelledAt().Equal(cancelledAt) {
		t.Fatalf("cancelledAt mapeado incorrectamente: %v", r.CancelledAt())
	}
}

// ─── FindByOwnerID ──────────────────────────────────────────────────────────

func TestReservationRepository_FindByOwnerID_ErrorDeQuery(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("owner-1").
		WillReturnError(errors.New("conexión perdida"))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindByOwnerID(context.Background(), "owner-1", "")
	assertInternalError(t, err)
}

func TestReservationRepository_FindByOwnerID_ErrorDeScan(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("owner-1").
		WillReturnRows(sqlmock.NewRows(reservationColumns).AddRow(
			nil, "prop-1", "tenant-1", "owner-1",
			time.Now(), time.Now(), 3, 10000.0, 0.0, 1500.0, 5000.0, 31500.0,
			"PENDING_APPROVAL", "", nil, nil, time.Now(), time.Now(),
		))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindByOwnerID(context.Background(), "owner-1", "")
	assertInternalError(t, err)
}

func TestReservationRepository_FindByOwnerID_ErrorDeIteracionDeFilas(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("owner-1").
		WillReturnRows(reservationRow("res-1", nil, nil).RowError(0, errors.New("fila corrupta")))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindByOwnerID(context.Background(), "owner-1", "")
	assertInternalError(t, err)
}

func TestReservationRepository_FindByOwnerID_SinFiltroDeStatus_NoAgregaCondicion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,.*FROM reservations\s+WHERE owner_id = \$1\s+ORDER BY created_at DESC`).
		WithArgs("owner-1").
		WillReturnRows(reservationRow("res-1", nil, nil))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindByOwnerID(context.Background(), "owner-1", "")
	if err != nil {
		t.Fatalf("FindByOwnerID: %v", err)
	}
	if len(result) != 1 || result[0].ID() != "res-1" {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_FindByOwnerID_ConFiltroDeStatus_AgregaCondicionYSegundoArg(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,.*FROM reservations\s+WHERE owner_id = \$1\s+AND status = \$2\s+ORDER BY created_at DESC`).
		WithArgs("owner-1", "CONFIRMED").
		WillReturnRows(reservationRow("res-1", nil, nil))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindByOwnerID(context.Background(), "owner-1", "CONFIRMED")
	if err != nil {
		t.Fatalf("FindByOwnerID: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_FindByOwnerID_ConfirmadaYCancelada_MapeaAmbosPunteros(t *testing.T) {
	db, mock := newMockDB(t)
	confirmedAt := time.Now().Add(-3 * time.Hour)
	cancelledAt := time.Now().Add(-1 * time.Hour)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("owner-1").
		WillReturnRows(reservationRow("res-1", &confirmedAt, &cancelledAt))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindByOwnerID(context.Background(), "owner-1", "")
	if err != nil {
		t.Fatalf("FindByOwnerID: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	r := result[0]
	if r.ConfirmedAt() == nil || !r.ConfirmedAt().Equal(confirmedAt) {
		t.Fatalf("confirmedAt mapeado incorrectamente: %v", r.ConfirmedAt())
	}
	if r.CancelledAt() == nil || !r.CancelledAt().Equal(cancelledAt) {
		t.Fatalf("cancelledAt mapeado incorrectamente: %v", r.CancelledAt())
	}
}

func TestReservationRepository_FindByOwnerID_SinResultados_RetornaSliceVacio(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WithArgs("owner-sin-reservas").
		WillReturnRows(sqlmock.NewRows(reservationColumns))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindByOwnerID(context.Background(), "owner-sin-reservas", "")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("esperaba slice vacío, obtuve %d elementos", len(result))
	}
}

// ─── HasOverlap ─────────────────────────────────────────────────────────────

func TestReservationRepository_HasOverlap_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("prop-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("timeout"))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.HasOverlap(context.Background(), "prop-1", time.Now(), time.Now().Add(24*time.Hour))
	assertInternalError(t, err)
}

func TestReservationRepository_HasOverlap_Existe(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("prop-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	repo := postgres.NewReservationRepository(db)

	overlap, err := repo.HasOverlap(context.Background(), "prop-1", time.Now(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("HasOverlap: %v", err)
	}
	if !overlap {
		t.Fatal("esperaba true")
	}
}

func TestReservationRepository_HasOverlap_NoExiste(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("prop-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	repo := postgres.NewReservationRepository(db)

	overlap, err := repo.HasOverlap(context.Background(), "prop-1", time.Now(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("HasOverlap: %v", err)
	}
	if overlap {
		t.Fatal("esperaba false")
	}
}
