package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// fakeReservationRepo implementa ports.ReservationRepository completo.
type fakeReservationRepo struct {
	saveFn                           func(ctx context.Context, r *domain.Reservation) error
	saveWithTxFn                     func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error
	findByIDFn                       func(ctx context.Context, id string) (*domain.Reservation, error)
	hasOverlapFn                     func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error)
	findByOwnerIDFn                  func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error)
	findConfirmedCheckingInBetweenFn func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error)
	markReminderSentFn               func(ctx context.Context, reservationID string) error
}

func (f *fakeReservationRepo) Save(ctx context.Context, r *domain.Reservation) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, r)
	}
	return errFixtureNoConfigurada
}
func (f *fakeReservationRepo) SaveWithTx(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error {
	if f.saveWithTxFn != nil {
		return f.saveWithTxFn(ctx, tx, r)
	}
	return errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindByID(ctx context.Context, id string) (*domain.Reservation, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) HasOverlap(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
	if f.hasOverlapFn != nil {
		return f.hasOverlapFn(ctx, propertyID, checkIn, checkOut)
	}
	return false, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindByOwnerID(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
	if f.findByOwnerIDFn != nil {
		return f.findByOwnerIDFn(ctx, ownerID, statusFilter)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindConfirmedCheckingInBetween(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
	if f.findConfirmedCheckingInBetweenFn != nil {
		return f.findConfirmedCheckingInBetweenFn(ctx, from, to)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) MarkReminderSent(ctx context.Context, reservationID string) error {
	if f.markReminderSentFn != nil {
		return f.markReminderSentFn(ctx, reservationID)
	}
	return errFixtureNoConfigurada
}

func pendingReservationFixture(t *testing.T) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 3, 10000, 0, 1500, 5000, 31500, "")
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}
	r.PullEvents()
	return r
}

func reservationWithStatus(t *testing.T, status domain.ReservationStatus) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	return domain.ReconstructReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 3, 10000, 0, 1500, 5000, 31500,
		status, "", nil, nil, time.Now(), time.Now())
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestCancelReservationUseCase_ErrorAlBuscarLaReserva_SePropagaTalCual(t *testing.T) {
	db, _ := newMockDB(t)
	dbErr := errors.New("timeout de base de datos")
	repo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, dbErr },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "tenant-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestCancelReservationUseCase_ReservaNoEncontrada_RetornaNotFound(t *testing.T) {
	db, _ := newMockDB(t)
	repo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-x", "tenant-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("got %v, want AppError NotFound", err)
	}
}

func TestCancelReservationUseCase_NiTenantNiOwner_RetornaForbidden(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "intruso")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("got %v, want AppError Forbidden", err)
	}
}

func TestCancelReservationUseCase_TenantPuedeCancelar(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	if err := uc.Execute(context.Background(), "res-1", "tenant-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if r.Status() != domain.ReservationCancelled {
		t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationCancelled)
	}
}

func TestCancelReservationUseCase_OwnerPuedeCancelar(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	if err := uc.Execute(context.Background(), "res-1", "owner-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if r.Status() != domain.ReservationCancelled {
		t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationCancelled)
	}
}

func TestCancelReservationUseCase_YaFinalizada_RetornaPreconditionFailed(t *testing.T) {
	db, _ := newMockDB(t)
	r := reservationWithStatus(t, domain.ReservationCompleted)
	repo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "tenant-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func TestCancelReservationUseCase_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "tenant-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func TestCancelReservationUseCase_ErrorEnSaveWithTx_SePropagaTalCual(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza, así que el rollback diferido se ejecuta de verdad.

	sentinel := apperr.NewInternal("fallo simulado de guardado", nil)
	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return sentinel },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "tenant-1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("esperaba que el error de SaveWithTx se propague tal cual: got %v", err)
	}
}

func TestCancelReservationUseCase_ErrorAlConfirmarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))

	r := pendingReservationFixture(t)
	repo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	uc := application.NewCancelReservationUseCase(db, repo)

	err := uc.Execute(context.Background(), "res-1", "tenant-1")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}
