package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func reservationFixtureWithID(t *testing.T, id, propertyID string) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	return domain.ReconstructReservation(id, propertyID, "tenant-1", "owner-1",
		checkIn, checkIn.Add(3*24*time.Hour), 3, 10000, 0, 1500, 5000, 31500,
		domain.ReservationPendingApproval, "", nil, nil, time.Now(), time.Now())
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestGetOwnerReservationsUseCase_OwnerIDVacio_RetornaBadRequest(t *testing.T) {
	uc := application.NewGetOwnerReservationsUseCase(&fakeReservationRepo{}, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "", "")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func TestGetOwnerReservationsUseCase_ErrorAlListar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			return nil, dbErr
		},
	}
	uc := application.NewGetOwnerReservationsUseCase(resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "owner-1", "")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestGetOwnerReservationsUseCase_SinReservas_RetornaSliceVacioNoNil(t *testing.T) {
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			return nil, nil
		},
	}
	uc := application.NewGetOwnerReservationsUseCase(resRepo, &fakeSnapshotRepo{})

	dtos, err := uc.Execute(context.Background(), "owner-1", "")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if dtos == nil {
		t.Fatal("esperaba un slice vacío no-nil (make con longitud 0), obtuve nil")
	}
	if len(dtos) != 0 {
		t.Fatalf("esperaba 0 elementos, obtuve %d", len(dtos))
	}
}

func TestGetOwnerReservationsUseCase_PropagaElStatusFilterAlRepo(t *testing.T) {
	var capturedFilter string
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			capturedFilter = statusFilter
			return nil, nil
		},
	}
	uc := application.NewGetOwnerReservationsUseCase(resRepo, &fakeSnapshotRepo{})

	if _, err := uc.Execute(context.Background(), "owner-1", "CONFIRMED"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedFilter != "CONFIRMED" {
		t.Fatalf("status filter: got %q, want %q", capturedFilter, "CONFIRMED")
	}
}

func TestGetOwnerReservationsUseCase_VariasReservas_BuscaElSnapshotDeCadaUna(t *testing.T) {
	r1 := reservationFixtureWithID(t, "res-1", "prop-1")
	r2 := reservationFixtureWithID(t, "res-2", "prop-2")

	var lookedUp []string
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r1, r2}, nil
		},
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
			lookedUp = append(lookedUp, propertyID)
			return &domain.PropertySnapshot{PropertyID: propertyID, CheckInTime: "14:00", CheckOutTime: "10:00"}, nil
		},
	}
	uc := application.NewGetOwnerReservationsUseCase(resRepo, snapRepo)

	dtos, err := uc.Execute(context.Background(), "owner-1", "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(dtos) != 2 {
		t.Fatalf("esperaba 2 DTOs, obtuve %d", len(dtos))
	}
	if dtos[0].ID != "res-1" || dtos[1].ID != "res-2" {
		t.Fatalf("orden/identidad de los DTOs incorrecta: %+v", dtos)
	}
	if len(lookedUp) != 2 || lookedUp[0] != "prop-1" || lookedUp[1] != "prop-2" {
		t.Fatalf("no se buscó el snapshot correcto para cada reserva: %v", lookedUp)
	}
	if dtos[0].CheckInTime != "14:00" || dtos[1].CheckInTime != "14:00" {
		t.Fatalf("horarios del snapshot no propagados: %+v", dtos)
	}
}

func TestGetOwnerReservationsUseCase_ErrorAlBuscarUnSnapshot_NoAbortaElListado(t *testing.T) {
	// Mismo patrón "mejor esfuerzo" que en Get/ConfirmReservationUseCase: si
	// el snapshot de una reserva falla, esa reserva igual se incluye en el
	// resultado, solo que sin CheckInTime/CheckOutTime.
	r1 := reservationFixtureWithID(t, "res-1", "prop-1")

	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r1}, nil
		},
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
			return nil, errors.New("timeout al buscar snapshot")
		},
	}
	uc := application.NewGetOwnerReservationsUseCase(resRepo, snapRepo)

	dtos, err := uc.Execute(context.Background(), "owner-1", "")
	if err != nil {
		t.Fatalf("no esperaba error aunque el snapshot falle: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("esperaba 1 DTO igual, obtuve %d", len(dtos))
	}
	if dtos[0].CheckInTime != "" || dtos[0].CheckOutTime != "" {
		t.Fatalf("esperaba horarios vacíos sin snapshot: %+v", dtos[0])
	}
}
