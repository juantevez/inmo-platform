package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"inmo.platform/contexts/maintenance/internal/domain"
)

type PostgresInquilinoProjectionRepository struct {
	db *sql.DB
}

func NewPostgresInquilinoProjectionRepository(db *sql.DB) *PostgresInquilinoProjectionRepository {
	return &PostgresInquilinoProjectionRepository{db: db}
}

// Upsert inserta o actualiza la proyección del inquilino.
// ON CONFLICT actualiza email y synced_at por si el evento llega duplicado.
func (r *PostgresInquilinoProjectionRepository) Upsert(ctx context.Context, i *domain.InquilinoProjection) error {
	query := `
		INSERT INTO inquilino_projections (user_id, email, status, synced_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			email     = EXCLUDED.email,
			status    = EXCLUDED.status,
			synced_at = EXCLUDED.synced_at
	`
	_, err := r.db.ExecContext(ctx, query,
		i.UserID(),
		i.Email(),
		i.Status(),
		i.SyncedAt(),
		i.CreatedAt(),
	)
	return err
}

// FindByUserID recupera la proyección por user_id.
// Retorna nil, nil si no existe.
func (r *PostgresInquilinoProjectionRepository) FindByUserID(ctx context.Context, userID string) (*domain.InquilinoProjection, error) {
	query := `
		SELECT user_id, email, status, synced_at, created_at
		FROM inquilino_projections
		WHERE user_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, userID)

	var uid, email, status string
	var syncedAt, createdAt time.Time

	err := row.Scan(&uid, &email, &status, &syncedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return domain.ReconstructInquilinoProjection(uid, email, status, syncedAt, createdAt), nil
}

// UpdateStatus actualiza solo el campo status de la proyección.
func (r *PostgresInquilinoProjectionRepository) UpdateStatus(ctx context.Context, userID, newStatus string) error {
	query := `
		UPDATE inquilino_projections
		SET status = $1, synced_at = $2
		WHERE user_id = $3
	`
	_, err := r.db.ExecContext(ctx, query, newStatus, time.Now(), userID)
	return err
}
