package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"inmo.platform/shared/pkg/apperr"
)

type BlockedDatesRepository struct {
	db *sql.DB
}

func NewBlockedDatesRepository(db *sql.DB) *BlockedDatesRepository {
	return &BlockedDatesRepository{db: db}
}

func (r *BlockedDatesRepository) HasOverlap(ctx context.Context, propertyID string, start, end time.Time) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM property_blocked_dates
			WHERE property_id = $1
			  AND start_date < $3
			  AND end_date   > $2
		)
	`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, propertyID, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&exists)
	if err != nil {
		return false, apperr.NewInternal("error al verificar disponibilidad", err)
	}
	return exists, nil
}

func (r *BlockedDatesRepository) Block(ctx context.Context, propertyID, reservationID, reason string, start, end time.Time) error {
	id := fmt.Sprintf("blk-%s-%d", propertyID, time.Now().UnixNano())
	var resID interface{}
	if reservationID != "" {
		resID = reservationID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO property_blocked_dates (id, property_id, start_date, end_date, reason, reservation_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING`,
		id, propertyID, start.Format("2006-01-02"), end.Format("2006-01-02"), reason, resID,
	)
	if err != nil {
		return apperr.NewInternal("error al bloquear fechas", err)
	}
	return nil
}

func (r *BlockedDatesRepository) Unblock(ctx context.Context, reservationID string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM property_blocked_dates WHERE reservation_id = $1", reservationID,
	)
	if err != nil {
		return apperr.NewInternal("error al desbloquear fechas", err)
	}
	return nil
}
