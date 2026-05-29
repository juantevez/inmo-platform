package postgres

import (
	"context"
	"database/sql"
	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type LeadRepository struct {
	db *sql.DB
}

func NewLeadRepository(db *sql.DB) *LeadRepository {
	return &LeadRepository{db: db}
}

func (r *LeadRepository) Save(ctx context.Context, l *domain.Lead) error {
	query := `
		INSERT INTO leads (id, property_id, client_name, email, phone, state, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			updated_at = CURRENT_TIMESTAMP;
	`

	_, err := r.db.ExecContext(ctx, query,
		l.ID(),
		l.PropertyID(),
		l.ClientName(),
		l.Email(),
		l.Phone(),
		string(l.State()),
	)

	if err != nil {
		return apperr.NewInternal("error al guardar el lead en postgres", err)
	}
	return nil
}
