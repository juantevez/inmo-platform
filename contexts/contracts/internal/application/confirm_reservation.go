package application

import (
	"context"
	"database/sql"

	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type ConfirmReservationUseCase struct {
	db       *sql.DB
	resRepo  ports.ReservationRepository
	snapRepo ports.PropertySnapshotRepository
}

func NewConfirmReservationUseCase(db *sql.DB, resRepo ports.ReservationRepository, snapRepo ports.PropertySnapshotRepository) *ConfirmReservationUseCase {
	return &ConfirmReservationUseCase{db: db, resRepo: resRepo, snapRepo: snapRepo}
}

func (uc *ConfirmReservationUseCase) Execute(ctx context.Context, reservationID, ownerID string) (*ReservationDTO, error) {
	reservation, err := uc.resRepo.FindByID(ctx, reservationID)
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, apperr.NewNotFound("reserva no encontrada", nil)
	}
	if reservation.OwnerID() != ownerID {
		return nil, apperr.NewForbidden("solo el propietario puede confirmar la reserva", nil)
	}

	if err := reservation.Confirm(); err != nil {
		return nil, err
	}

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperr.NewInternal("error al iniciar transacción", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := uc.resRepo.SaveWithTx(ctx, tx, reservation); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, apperr.NewInternal("error al confirmar transacción", err)
	}

	snap, _ := uc.snapRepo.FindByID(ctx, reservation.PropertyID())
	return toReservationDTO(reservation, snap), nil
}
