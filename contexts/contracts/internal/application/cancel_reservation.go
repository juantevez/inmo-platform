package application

import (
	"context"
	"database/sql"

	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type CancelReservationUseCase struct {
	db      *sql.DB
	resRepo ports.ReservationRepository
}

func NewCancelReservationUseCase(db *sql.DB, resRepo ports.ReservationRepository) *CancelReservationUseCase {
	return &CancelReservationUseCase{db: db, resRepo: resRepo}
}

func (uc *CancelReservationUseCase) Execute(ctx context.Context, reservationID, requesterID string) error {
	reservation, err := uc.resRepo.FindByID(ctx, reservationID)
	if err != nil {
		return err
	}
	if reservation == nil {
		return apperr.NewNotFound("reserva no encontrada", nil)
	}
	if reservation.TenantID() != requesterID && reservation.OwnerID() != requesterID {
		return apperr.NewForbidden("solo el inquilino o el propietario pueden cancelar la reserva", nil)
	}

	if err := reservation.Cancel(); err != nil {
		return err
	}

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.NewInternal("error al iniciar transacción", err)
	}
	defer tx.Rollback()

	if err := uc.resRepo.SaveWithTx(ctx, tx, reservation); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return apperr.NewInternal("error al confirmar transacción", err)
	}
	return nil
}
