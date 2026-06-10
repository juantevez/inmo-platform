package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"inmo.platform/contexts/maintenance/internal/domain"
)

type PostgresProjectionRepository struct {
	db *sql.DB
}

func NewPostgresProjectionRepository(db *sql.DB) *PostgresProjectionRepository {
	return &PostgresProjectionRepository{db: db}
}

// Upsert inserta o actualiza la proyección local de una propiedad.
// Se llama cada vez que llega catalog.property.published o catalog.property.state_changed.
func (r *PostgresProjectionRepository) Upsert(ctx context.Context, p *domain.PropertyProjection) error {
	query := `
		INSERT INTO property_projections (
			property_id, owner_id, operation_type, state, tenant_id, synced_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (property_id) DO UPDATE SET
			owner_id       = EXCLUDED.owner_id,
			operation_type = EXCLUDED.operation_type,
			state          = EXCLUDED.state,
			tenant_id      = EXCLUDED.tenant_id,
			synced_at      = EXCLUDED.synced_at
	`
	_, err := r.db.ExecContext(ctx, query,
		p.PropertyID(),
		p.OwnerID(),
		p.OperationType(),
		p.State(),
		nullableString(p.TenantID()),
		p.SyncedAt(),
		p.CreatedAt(),
	)
	return err
}

// FindByID recupera la proyección local de una propiedad por su ID.
// Retorna nil, nil si no existe — indica que la propiedad no fue publicada aún.
func (r *PostgresProjectionRepository) FindByID(ctx context.Context, propertyID string) (*domain.PropertyProjection, error) {
	query := `
		SELECT property_id, owner_id, operation_type, state, tenant_id, synced_at, created_at
		FROM property_projections
		WHERE property_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, propertyID)

	var (
		propID, ownerID, opType, state string
		tenantID                       sql.NullString
		syncedAt, createdAt            time.Time
	)

	err := row.Scan(&propID, &ownerID, &opType, &state, &tenantID, &syncedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return domain.ReconstructPropertyProjection(
		propID,
		ownerID,
		opType,
		state,
		tenantID.String,
		syncedAt,
		createdAt,
	), nil
}

// UpdateState actualiza solo el campo state de la proyección.
// Más eficiente que Upsert completo para cambios de estado frecuentes.
func (r *PostgresProjectionRepository) UpdateState(ctx context.Context, propertyID, newState string) error {
	query := `
		UPDATE property_projections
		SET state = $1, synced_at = $2
		WHERE property_id = $3
	`
	_, err := r.db.ExecContext(ctx, query, newState, time.Now(), propertyID)
	return err
}
