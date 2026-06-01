package application

import (
	"context"

	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type GetReservationUseCase struct {
	resRepo  ports.ReservationRepository
	snapRepo ports.PropertySnapshotRepository
}

func NewGetReservationUseCase(resRepo ports.ReservationRepository, snapRepo ports.PropertySnapshotRepository) *GetReservationUseCase {
	return &GetReservationUseCase{resRepo: resRepo, snapRepo: snapRepo}
}

func (uc *GetReservationUseCase) Execute(ctx context.Context, reservationID string) (*ReservationDTO, error) {
	reservation, err := uc.resRepo.FindByID(ctx, reservationID)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, apperr.NewNotFound("reserva no encontrada", nil)
	}

	snap, _ := uc.snapRepo.FindByID(ctx, reservation.PropertyID())
	return toReservationDTO(reservation, snap), nil
}
