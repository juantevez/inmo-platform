package application

import (
	"context"

	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// GetOwnerReservationsUseCase devuelve todas las reservas donde owner_id = el usuario autenticado.
// Acepta un filtro opcional de status (vacío = todas).
type GetOwnerReservationsUseCase struct {
	resRepo  ports.ReservationRepository
	snapRepo ports.PropertySnapshotRepository
}

func NewGetOwnerReservationsUseCase(
	resRepo ports.ReservationRepository,
	snapRepo ports.PropertySnapshotRepository,
) *GetOwnerReservationsUseCase {
	return &GetOwnerReservationsUseCase{resRepo: resRepo, snapRepo: snapRepo}
}

func (uc *GetOwnerReservationsUseCase) Execute(ctx context.Context, ownerID, statusFilter string) ([]*ReservationDTO, error) {
	if ownerID == "" {
		return nil, apperr.NewBadRequest("owner_id es requerido", nil)
	}

	reservations, err := uc.resRepo.FindByOwnerID(ctx, ownerID, statusFilter)
	if err != nil {
		return nil, err
	}

	dtos := make([]*ReservationDTO, 0, len(reservations))
	for _, r := range reservations {
		snap, _ := uc.snapRepo.FindByID(ctx, r.PropertyID())
		dtos = append(dtos, toReservationDTO(r, snap))
	}

	return dtos, nil
}
