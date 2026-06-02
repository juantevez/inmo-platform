package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type ReservationRepository struct {
	db *sql.DB
}

func NewReservationRepository(db *sql.DB) *ReservationRepository {
	return &ReservationRepository{db: db}
}

func (r *ReservationRepository) Save(ctx context.Context, res *domain.Reservation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.NewInternal("error al iniciar transacción", err)
	}
	defer tx.Rollback()
	if err := r.SaveWithTx(ctx, tx, res); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ReservationRepository) SaveWithTx(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error {
	query := `
		INSERT INTO reservations (
			id, property_id, tenant_id, owner_id,
			check_in_date, check_out_date, nights,
			night_price_snapshot, discount_pct, cleaning_fee, security_deposit, total_amount,
			status, guest_message, confirmed_at, cancelled_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (id) DO UPDATE SET
			status=EXCLUDED.status, confirmed_at=EXCLUDED.confirmed_at,
			cancelled_at=EXCLUDED.cancelled_at, updated_at=EXCLUDED.updated_at;
	`
	_, err := tx.ExecContext(ctx, query,
		res.ID(), res.PropertyID(), res.TenantID(), res.OwnerID(),
		res.CheckInDate().Format("2006-01-02"), res.CheckOutDate().Format("2006-01-02"), res.Nights(),
		res.NightPriceSnapshot(), res.DiscountPct(), res.CleaningFee(), res.SecurityDeposit(), res.TotalAmount(),
		string(res.Status()), nullStr(res.GuestMessage()), res.ConfirmedAt(), res.CancelledAt(),
		res.CreatedAt(), res.UpdatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar reserva", err)
	}

	for _, event := range res.PullEvents() {
		payload, err := json.Marshal(event)
		if err != nil {
			return apperr.NewInternal("error al serializar evento de reserva", err)
		}
		evtID := fmt.Sprintf("evt-res-%s-%d", res.ID(), time.Now().UnixNano())
		_, err = tx.ExecContext(ctx, `
			INSERT INTO contracts_outbox_events (id, aggregate_type, aggregate_id, event_name, payload, status)
			VALUES ($1, $2, $3, $4, $5, 'PENDING')`,
			evtID, "Reservation", res.ID(), event.EventName(), payload,
		)
		if err != nil {
			return apperr.NewInternal("error al insertar evento de reserva en outbox", err)
		}
	}
	return nil
}

func (r *ReservationRepository) FindByID(ctx context.Context, id string) (*domain.Reservation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, property_id, tenant_id, owner_id,
		       check_in_date, check_out_date, nights,
		       night_price_snapshot, discount_pct, cleaning_fee, security_deposit, total_amount,
		       status, guest_message, confirmed_at, cancelled_at, created_at, updated_at
		FROM reservations WHERE id = $1`, id)

	var (
		resID, propID, tenantID, ownerID, status     string
		checkIn, checkOut                             time.Time // DATE → time.Time directo
		nights                                        int
		nightPrice, discPct, cleaning, deposit, total float64
		guestMsg                                      sql.NullString
		confirmedAt, cancelledAt                      sql.NullTime
		createdAt, updatedAt                          time.Time
	)
	err := row.Scan(
		&resID, &propID, &tenantID, &ownerID,
		&checkIn, &checkOut, &nights,
		&nightPrice, &discPct, &cleaning, &deposit, &total,
		&status, &guestMsg, &confirmedAt, &cancelledAt, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al buscar reserva", err)
	}

	ci := checkIn
	co := checkOut
	var confAt, canAt *time.Time
	if confirmedAt.Valid {
		t := confirmedAt.Time
		confAt = &t
	}
	if cancelledAt.Valid {
		t := cancelledAt.Time
		canAt = &t
	}

	return domain.ReconstructReservation(
		resID, propID, tenantID, ownerID, ci, co, nights,
		nightPrice, discPct, cleaning, deposit, total,
		domain.ReservationStatus(status), guestMsg.String,
		confAt, canAt, createdAt, updatedAt,
	), nil
}

func (r *ReservationRepository) HasOverlap(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM reservations
			WHERE property_id = $1
			  AND status IN ('PENDING_APPROVAL','CONFIRMED','ACTIVE')
			  AND check_in_date  < $3
			  AND check_out_date > $2
		)`,
		propertyID, checkIn.Format("2006-01-02"), checkOut.Format("2006-01-02"),
	).Scan(&exists)
	if err != nil {
		return false, apperr.NewInternal("error al verificar solapamiento de reservas", err)
	}
	return exists, nil
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
