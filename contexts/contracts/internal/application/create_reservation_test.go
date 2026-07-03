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

func snapshotFixture(minNights, maxNights int) *domain.PropertySnapshot {
	return &domain.PropertySnapshot{
		PropertyID:      "prop-1",
		OwnerID:         "owner-1",
		OperationType:   "TEMP",
		NightPrice:      10000,
		CleaningFee:     1500,
		SecurityDeposit: 5000,
		MinNights:       minNights,
		MaxNights:       maxNights,
		CheckInTime:     "14:00",
		CheckOutTime:    "10:00",
	}
}

func validCreateReservationCmd() application.CreateReservationCommand {
	checkIn := time.Now().Add(24 * time.Hour)
	return application.CreateReservationCommand{
		PropertyID:   "prop-1",
		TenantID:     "tenant-1",
		CheckInDate:  checkIn,
		CheckOutDate: checkIn.Add(3 * 24 * time.Hour),
		GuestMessage: "Llegamos tarde",
	}
}

func assertPreconditionFailed(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("got %v, want AppError NotFound", err)
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestCreateReservationUseCase_ErrorAlBuscarElSnapshot_SePropagaTalCual(t *testing.T) {
	db, _ := newMockDB(t)
	dbErr := errors.New("timeout de base de datos")
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return nil, dbErr },
	}
	uc := application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestCreateReservationUseCase_PropiedadNoEncontrada_RetornaNotFound(t *testing.T) {
	db, _ := newMockDB(t)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return nil, nil },
	}
	uc := application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	assertNotFound(t, err)
}

func TestCreateReservationUseCase_MenosQueElMinimo_RetornaBadRequest(t *testing.T) {
	db, _ := newMockDB(t)
	snap := snapshotFixture(5, 30) // pide mínimo 5 noches
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	uc := application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo)

	cmd := validCreateReservationCmd() // 3 noches
	_, err := uc.Execute(context.Background(), cmd)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func TestCreateReservationUseCase_MasQueElMaximo_RetornaBadRequest(t *testing.T) {
	db, _ := newMockDB(t)
	snap := snapshotFixture(1, 2) // máximo 2 noches
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	uc := application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo)

	cmd := validCreateReservationCmd() // 3 noches
	_, err := uc.Execute(context.Background(), cmd)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func TestCreateReservationUseCase_MaxNightsCero_SignificaSinLimite(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	snap := snapshotFixture(1, 0) // sin límite superior
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { return nil },
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	checkIn := time.Now().Add(24 * time.Hour)
	cmd := application.CreateReservationCommand{
		PropertyID: "prop-1", TenantID: "tenant-1",
		CheckInDate: checkIn, CheckOutDate: checkIn.Add(90 * 24 * time.Hour), // 90 noches
	}
	dto, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("no esperaba error con MaxNights=0 (sin límite): %v", err)
	}
	if dto.Nights != 90 {
		t.Fatalf("nights: got %d, want 90", dto.Nights)
	}
}

func TestCreateReservationUseCase_NochesCeroConMinimoCero_FallaEnElDominio(t *testing.T) {
	// Caso límite: si MinNights=0 y la estadía dura menos de 24hs, nights=0
	// pasa la validación "nights < MinNights" (0 < 0 es falso) pero
	// domain.NewReservation la rechaza igual (exige al menos 1 noche).
	db, _ := newMockDB(t)
	snap := snapshotFixture(0, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	checkIn := time.Now().Add(24 * time.Hour)
	cmd := application.CreateReservationCommand{
		PropertyID: "prop-1", TenantID: "tenant-1",
		CheckInDate: checkIn, CheckOutDate: checkIn.Add(12 * time.Hour), // menos de 1 noche completa
	}
	_, err := uc.Execute(context.Background(), cmd)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest (rechazado por el dominio)", err)
	}
}

func TestCreateReservationUseCase_ErrorAlVerificarSolapamiento_SePropagaTalCual(t *testing.T) {
	db, _ := newMockDB(t)
	dbErr := errors.New("timeout de red")
	snap := snapshotFixture(1, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, dbErr
		},
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestCreateReservationUseCase_FechasSuperpuestas_RetornaPreconditionFailed(t *testing.T) {
	db, _ := newMockDB(t)
	snap := snapshotFixture(1, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return true, nil
		},
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	assertPreconditionFailed(t, err)
}

func TestCreateReservationUseCase_ErrorAlIniciarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	snap := snapshotFixture(1, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func TestCreateReservationUseCase_ErrorEnSaveWithTx_SePropagaTalCual(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectRollback() // Commit() nunca se alcanza, así que el rollback diferido se ejecuta de verdad.

	sentinel := apperr.NewInternal("fallo simulado de guardado", nil)
	snap := snapshotFixture(1, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { return sentinel },
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	if !errors.Is(err, sentinel) {
		t.Fatalf("esperaba que el error de SaveWithTx se propague tal cual: got %v", err)
	}
}

func TestCreateReservationUseCase_ErrorAlConfirmarTransaccion(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("fallo de disco"))

	snap := snapshotFixture(1, 30)
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { return nil },
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	_, err := uc.Execute(context.Background(), validCreateReservationCmd())
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func TestCreateReservationUseCase_Exitoso_CalculaDescuentoYRetornaDTO(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	snap := snapshotFixture(1, 30)
	snap.PricingRules = []domain.PricingRule{{Type: "weekly", MinNights: 7, DiscountPct: 10}}

	var saved *domain.Reservation
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { saved = r; return nil },
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	checkIn := time.Now().Add(24 * time.Hour)
	cmd := application.CreateReservationCommand{
		PropertyID: "prop-1", TenantID: "tenant-1",
		CheckInDate: checkIn, CheckOutDate: checkIn.Add(7 * 24 * time.Hour), // 7 noches: aplica la regla weekly
		GuestMessage: "Hola!",
	}
	dto, err := uc.Execute(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// subtotal = 10000*7 = 70000; descuento 10% = 7000; + cleaning 1500 = 64500
	if dto.TotalAmount != 64500 {
		t.Fatalf("total amount: got %v, want 64500", dto.TotalAmount)
	}
	if dto.DiscountPct != 10 {
		t.Fatalf("discount pct: got %v, want 10", dto.DiscountPct)
	}
	if dto.Nights != 7 {
		t.Fatalf("nights: got %d, want 7", dto.Nights)
	}
	if dto.OwnerID != "owner-1" {
		t.Fatalf("owner id: got %s, want owner-1", dto.OwnerID)
	}
	if dto.CheckInTime != "14:00" || dto.CheckOutTime != "10:00" {
		t.Fatalf("horarios del snapshot no propagados al DTO: %+v", dto)
	}
	if dto.Status != string(domain.ReservationPendingApproval) {
		t.Fatalf("status: got %s, want %s", dto.Status, domain.ReservationPendingApproval)
	}
	if saved == nil || saved.PropertyID() != "prop-1" || saved.TenantID() != "tenant-1" || saved.OwnerID() != "owner-1" {
		t.Fatalf("reserva guardada incorrectamente: %+v", saved)
	}
}

func TestCreateReservationUseCase_SinReglasDeDescuento_TotalSinDescuento(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	snap := snapshotFixture(1, 30) // sin PricingRules

	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil },
	}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { return nil },
	}
	uc := application.NewCreateReservationUseCase(db, resRepo, snapRepo)

	dto, err := uc.Execute(context.Background(), validCreateReservationCmd()) // 3 noches
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// subtotal = 10000*3 = 30000; sin descuento; + cleaning 1500 = 31500
	if dto.TotalAmount != 31500 {
		t.Fatalf("total amount: got %v, want 31500", dto.TotalAmount)
	}
	if dto.DiscountPct != 0 {
		t.Fatalf("discount pct: got %v, want 0", dto.DiscountPct)
	}
}
