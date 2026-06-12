package postgres

// ── Agregar estos dos métodos al final de reservation_repository.go ──────────
// (el resto del archivo queda idéntico)

import (
	"context"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// FindConfirmedCheckingInBetween implementa el puerto requerido por ReminderScheduler.
// Devuelve reservas CONFIRMED con check-in en la ventana [from, to] que aún no
// tuvieron el recordatorio enviado.
func (r *ReservationRepository) FindConfirmedCheckingInBetween(
	ctx context.Context,
	from, to time.Time,
) ([]*domain.Reservation, error) {

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, property_id, tenant_id, owner_id,
		       check_in_date, check_out_date, nights,
		       night_price_snapshot, discount_pct, cleaning_fee, security_deposit, total_amount,
		       status, guest_message, confirmed_at, cancelled_at, created_at, updated_at
		FROM reservations
		WHERE status        = 'CONFIRMED'
		  AND reminder_sent = FALSE
		  AND check_in_date >= $1::date
		  AND check_in_date <= $2::date
		ORDER BY check_in_date ASC`,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, apperr.NewInternal("error al buscar reservas próximas al check-in", err)
	}
	defer rows.Close()

	// Reutilizamos el mismo scanner que usa FindByOwnerID
	return scanReservationRows(rows)
}

// MarkReminderSent implementa el puerto requerido por ReminderScheduler.
func (r *ReservationRepository) MarkReminderSent(ctx context.Context, reservationID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE reservations SET reminder_sent = TRUE, updated_at = NOW() WHERE id = $1`,
		reservationID,
	)
	if err != nil {
		return apperr.NewInternal("error al marcar reminder_sent", err)
	}
	return nil
}
