package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/adapters/postgres"
)

// Nota: scanReservationRows (reservation_repository_scan.go) es un helper no
// exportado cuyo único caller actual es FindConfirmedCheckingInBetween — según
// el comentario de migración en reservation_repository_scan.go, FindByOwnerID
// todavía no fue refactorizado para reutilizarlo y mantiene su propio bucle
// duplicado. Los tests de abajo cubren scanReservationRows indirectamente a
// través de su único punto de entrada real.

// ─── FindConfirmedCheckingInBetween ─────────────────────────────────────────

func TestReservationRepository_FindConfirmedCheckingInBetween_ErrorDeQuery(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WillReturnError(errors.New("timeout de red"))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindConfirmedCheckingInBetween(context.Background(), time.Now(), time.Now().Add(24*time.Hour))
	assertInternalError(t, err)
}

func TestReservationRepository_FindConfirmedCheckingInBetween_ErrorDeScan(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WillReturnRows(sqlmock.NewRows(reservationColumns).AddRow(
			nil, "prop-1", "tenant-1", "owner-1",
			time.Now(), time.Now(), 3, 10000.0, 0.0, 1500.0, 5000.0, 31500.0,
			"CONFIRMED", "", nil, nil, time.Now(), time.Now(),
		))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindConfirmedCheckingInBetween(context.Background(), time.Now(), time.Now().Add(24*time.Hour))
	assertInternalError(t, err)
}

func TestReservationRepository_FindConfirmedCheckingInBetween_ErrorDeIteracionDeFilas(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WillReturnRows(reservationRow("res-1", nil, nil).RowError(0, errors.New("fila corrupta")))
	repo := postgres.NewReservationRepository(db)

	_, err := repo.FindConfirmedCheckingInBetween(context.Background(), time.Now(), time.Now().Add(24*time.Hour))
	assertInternalError(t, err)
}

func TestReservationRepository_FindConfirmedCheckingInBetween_Exitoso_UsaElRangoDeFechasFormateado(t *testing.T) {
	db, mock := newMockDB(t)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,.*WHERE status\s+= 'CONFIRMED'`).
		WithArgs("2026-08-01", "2026-08-07").
		WillReturnRows(reservationRow("res-1", nil, nil))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindConfirmedCheckingInBetween(context.Background(), from, to)
	if err != nil {
		t.Fatalf("FindConfirmedCheckingInBetween: %v", err)
	}
	if len(result) != 1 || result[0].ID() != "res-1" {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationRepository_FindConfirmedCheckingInBetween_SinResultados(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WillReturnRows(sqlmock.NewRows(reservationColumns))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindConfirmedCheckingInBetween(context.Background(), time.Now(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("esperaba slice vacío, obtuve %d elementos", len(result))
	}
}

func TestReservationRepository_FindConfirmedCheckingInBetween_ConConfirmedAtYCancelledAt(t *testing.T) {
	db, mock := newMockDB(t)
	confirmedAt := time.Now().Add(-24 * time.Hour)
	cancelledAt := time.Now().Add(-1 * time.Hour)
	mock.ExpectQuery(`SELECT id, property_id, tenant_id, owner_id,`).
		WillReturnRows(reservationRow("res-1", &confirmedAt, &cancelledAt))
	repo := postgres.NewReservationRepository(db)

	result, err := repo.FindConfirmedCheckingInBetween(context.Background(), time.Now(), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("FindConfirmedCheckingInBetween: %v", err)
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

// ─── MarkReminderSent ───────────────────────────────────────────────────────

func TestReservationRepository_MarkReminderSent_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE reservations SET reminder_sent = TRUE`).
		WithArgs("res-1").
		WillReturnError(errors.New("fallo de escritura"))
	repo := postgres.NewReservationRepository(db)

	err := repo.MarkReminderSent(context.Background(), "res-1")
	assertInternalError(t, err)
}

func TestReservationRepository_MarkReminderSent_Exitoso(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE reservations SET reminder_sent = TRUE`).
		WithArgs("res-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := postgres.NewReservationRepository(db)

	if err := repo.MarkReminderSent(context.Background(), "res-1"); err != nil {
		t.Fatalf("MarkReminderSent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
