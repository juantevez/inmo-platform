package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

type fakeSnapshotRepo struct {
	upsertFn   func(ctx context.Context, snap domain.PropertySnapshot) error
	findByIDFn func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error)
}

func (f *fakeSnapshotRepo) Upsert(ctx context.Context, snap domain.PropertySnapshot) error {
	if f.upsertFn != nil {
		return f.upsertFn(ctx, snap)
	}
	return errFixtureNoConfigurada
}
func (f *fakeSnapshotRepo) FindByID(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, propertyID)
	}
	return nil, errFixtureNoConfigurada
}

func validSnapshotFixture() *domain.PropertySnapshot {
	return &domain.PropertySnapshot{
		PropertyID:   "prop-1",
		OwnerID:      "owner-1",
		NightPrice:   10000,
		CheckInTime:  "14:00",
		CheckOutTime: "10:00",
		MinNights:    2,
		MaxNights:    30,
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestConfirmReservationUseCase_ErrorAlBuscarLaReserva_SePropagaTalCual(t *testing.T) {
	db, _ := newMockDB(t)
	dbErr := errors.New("timeout de base de datos")
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, dbErr },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "owner-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestConfirmReservationUseCase_NoEncontrada_RetornaNotFound(t *testing.T) {
	db, _ := newMockDB(t)
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-x", "owner-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("got %v, want AppError NotFound", err)
	}
}

func TestConfirmReservationUseCase_NoEsElPropietario_RetornaForbidden(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservationFixture(t)
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "otro-usuario")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("got %v, want AppError Forbidden", err)
	}
}

func TestConfirmReservationUseCase_EstadoNoPermiteConfirmar_SePropagaTalCual(t *testing.T) {
	db, _ := newMockDB(t)
	r := reservationWithStatus(t, domain.ReservationConfirmed) // ya confirmada: Confirm() debe fallar
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "owner-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func TestConfirmReservationUseCase_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	r := pendingReservationFixture(t)
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "owner-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func TestConfirmReservationUseCase_ErrorEnSaveWithTx_SePropagaTalCual(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza, así que el rollback diferido se ejecuta de verdad.

	sentinel := apperr.NewInternal("fallo simulado de guardado", nil)
	r := pendingReservationFixture(t)
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return sentinel },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "owner-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("esperaba que el error de SaveWithTx se propague tal cual: got %v", err)
	}
}

func TestConfirmReservationUseCase_ErrorAlConfirmarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))

	r := pendingReservationFixture(t)
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1", "owner-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func TestConfirmReservationUseCase_Exitoso_ConfirmaYRetornaDTOEnriquecido(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservationFixture(t)
	snap := validSnapshotFixture()
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, snapRepo)

	dto, err := uc.Execute(context.Background(), "res-1", "owner-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if r.Status() != domain.ReservationConfirmed {
		t.Fatalf("status del agregado: got %s, want %s", r.Status(), domain.ReservationConfirmed)
	}
	if dto.Status != string(domain.ReservationConfirmed) {
		t.Fatalf("status en el DTO: got %s", dto.Status)
	}
	if dto.CheckInTime != "14:00" || dto.CheckOutTime != "10:00" {
		t.Fatalf("horarios del snapshot no propagados al DTO: %+v", dto)
	}
}

func TestConfirmReservationUseCase_SnapshotNoEncontrado_IgualRetornaElDTO(t *testing.T) {
	// El lookup del snapshot es "mejor esfuerzo": si falla o no existe, la
	// confirmación NO se aborta — el DTO simplemente queda sin CheckInTime/
	// CheckOutTime. Mismo patrón usado en GetReservationUseCase.
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservationFixture(t)
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
			return nil, errors.New("timeout al buscar snapshot")
		},
	}
	uc := application.NewConfirmReservationUseCase(db, resRepo, snapRepo)

	dto, err := uc.Execute(context.Background(), "res-1", "owner-1")
	if err != nil {
		t.Fatalf("no esperaba error aunque el snapshot falle: %v", err)
	}
	if dto.CheckInTime != "" || dto.CheckOutTime != "" {
		t.Fatalf("esperaba horarios vacíos sin snapshot: %+v", dto)
	}
	if dto.ID != "res-1" {
		t.Fatalf("id del DTO: got %s", dto.ID)
	}
}
