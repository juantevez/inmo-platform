package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type SnapshotRepository struct {
	db *sql.DB
}

func NewSnapshotRepository(db *sql.DB) *SnapshotRepository {
	return &SnapshotRepository{db: db}
}

func (r *SnapshotRepository) Upsert(ctx context.Context, snap domain.PropertySnapshot) error {
	rulesJSON, err := json.Marshal(snap.PricingRules)
	if err != nil {
		return apperr.NewInternal("error al serializar pricing_rules del snapshot", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO property_snapshots (
			property_id, owner_id, operation_type, night_price, cleaning_fee, security_deposit,
			min_nights, max_nights, check_in_time, check_out_time, pricing_rules, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (property_id) DO UPDATE SET
			owner_id=EXCLUDED.owner_id, operation_type=EXCLUDED.operation_type,
			night_price=EXCLUDED.night_price, cleaning_fee=EXCLUDED.cleaning_fee,
			security_deposit=EXCLUDED.security_deposit, min_nights=EXCLUDED.min_nights,
			max_nights=EXCLUDED.max_nights, check_in_time=EXCLUDED.check_in_time,
			check_out_time=EXCLUDED.check_out_time, pricing_rules=EXCLUDED.pricing_rules,
			updated_at=EXCLUDED.updated_at`,
		snap.PropertyID, snap.OwnerID, snap.OperationType,
		snap.NightPrice, snap.CleaningFee, snap.SecurityDeposit,
		snap.MinNights, snap.MaxNights, snap.CheckInTime, snap.CheckOutTime,
		rulesJSON, time.Now().UTC(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar snapshot de propiedad", err)
	}
	return nil
}

func (r *SnapshotRepository) FindByID(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT property_id, owner_id, operation_type, night_price, cleaning_fee, security_deposit,
		       min_nights, max_nights, check_in_time, check_out_time, pricing_rules, updated_at
		FROM property_snapshots WHERE property_id = $1`, propertyID)

	var (
		propID, ownerID, opType, checkIn, checkOut string
		nightPrice, cleaningFee, securityDeposit   float64
		minNights, maxNights                       int
		rulesJSON                                  []byte
		updatedAt                                  time.Time
	)
	err := row.Scan(&propID, &ownerID, &opType, &nightPrice, &cleaningFee, &securityDeposit,
		&minNights, &maxNights, &checkIn, &checkOut, &rulesJSON, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al buscar snapshot de propiedad", err)
	}

	var rules []domain.PricingRule
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &rules); err != nil {
			return nil, apperr.NewInternal("datos corrompidos en pricing_rules del snapshot", err)
		}
	}

	return &domain.PropertySnapshot{
		PropertyID: propID, OwnerID: ownerID, OperationType: opType,
		NightPrice: nightPrice, CleaningFee: cleaningFee, SecurityDeposit: securityDeposit,
		MinNights: minNights, MaxNights: maxNights,
		CheckInTime: checkIn, CheckOutTime: checkOut,
		PricingRules: rules, UpdatedAt: updatedAt,
	}, nil
}
