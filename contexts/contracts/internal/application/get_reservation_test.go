package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
)

func TestGetReservationUseCase_ErrorAlBuscar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, dbErr },
	}
	uc := application.NewGetReservationUseCase(resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestGetReservationUseCase_NoEncontrada_RetornaNotFound(t *testing.T) {
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, nil },
	}
	uc := application.NewGetReservationUseCase(resRepo, &fakeSnapshotRepo{})

	_, err := uc.Execute(context.Background(), "res-x")
	assertNotFound(t, err)
}

func TestGetReservationUseCase_Exitoso_ConSnapshot(t *testing.T) {
	r := reservationFixtureWithID(t, "res-1", "prop-1")
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
			return &domain.PropertySnapshot{PropertyID: propertyID, CheckInTime: "14:00", CheckOutTime: "10:00"}, nil
		},
	}
	uc := application.NewGetReservationUseCase(resRepo, snapRepo)

	dto, err := uc.Execute(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if dto.ID != "res-1" || dto.PropertyID != "prop-1" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", dto)
	}
	if dto.CheckInTime != "14:00" || dto.CheckOutTime != "10:00" {
		t.Fatalf("horarios del snapshot no propagados al DTO: %+v", dto)
	}
}

func TestGetReservationUseCase_ErrorAlBuscarElSnapshot_IgualRetornaElDTO(t *testing.T) {
	// Patrón "mejor esfuerzo": el error del snapshot se descarta, la reserva
	// se retorna igual, solo sin CheckInTime/CheckOutTime.
	r := reservationFixtureWithID(t, "res-1", "prop-1")
	resRepo := &fakeReservationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
	}
	snapRepo := &fakeSnapshotRepo{
		findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
			return nil, errors.New("timeout al buscar snapshot")
		},
	}
	uc := application.NewGetReservationUseCase(resRepo, snapRepo)

	dto, err := uc.Execute(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("no esperaba error aunque el snapshot falle: %v", err)
	}
	if dto.ID != "res-1" {
		t.Fatalf("id del DTO: got %s", dto.ID)
	}
	if dto.CheckInTime != "" || dto.CheckOutTime != "" {
		t.Fatalf("esperaba horarios vacíos sin snapshot: %+v", dto)
	}
}
