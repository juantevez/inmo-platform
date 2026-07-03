package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type PostgresLeadRepository struct {
	db *sql.DB
}

func NewPostgresLeadRepository(db *sql.DB) ports.LeadRepository {
	return &PostgresLeadRepository{db: db}
}

func (r *PostgresLeadRepository) Save(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
	// Iniciamos la transacción estándar
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.NewInternal("no se pudo iniciar la transacción para guardar el lead", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Insertar o Actualizar el Lead (Upsert)
	leadQuery := `
        INSERT INTO leads (id, property_id, client_name, email, phone, state, visit_scheduled_at, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (id) DO UPDATE SET
            client_name = EXCLUDED.client_name,
            email = EXCLUDED.email,
            phone = EXCLUDED.phone,
            state = EXCLUDED.state,
            visit_scheduled_at = EXCLUDED.visit_scheduled_at,
            updated_at = EXCLUDED.updated_at;
    `

	_, err = tx.ExecContext(ctx, leadQuery,
		lead.ID,
		lead.PropertyID,
		lead.ClientName,
		sql.NullString{String: lead.Email, Valid: lead.Email != ""},
		sql.NullString{String: lead.Phone, Valid: lead.Phone != ""},
		string(lead.State),
		nullTimeFromPtr(lead.VisitScheduledAt),
		lead.CreatedAt,
		lead.UpdatedAt,
	)
	if err != nil {
		return apperr.NewInternal("error al persistir el lead", err)
	}

	// 2. Outbox
	if eventName != "" && len(eventPayload) > 0 {
		outboxQuery := `
            INSERT INTO crm_outbox_events (event_name, payload, status, created_at)
            VALUES ($1, $2, 'PENDING', $3);
        `
		_, err = tx.ExecContext(ctx, outboxQuery, eventName, eventPayload, time.Now())
		if err != nil {
			return apperr.NewInternal("error al enlistar evento outbox", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return apperr.NewInternal("error al confirmar la transacción del lead", err)
	}
	return nil
}

func (r *PostgresLeadRepository) GetByID(ctx context.Context, id string) (*domain.Lead, error) {
	query := `SELECT id, property_id, client_name, email, phone, state, visit_scheduled_at, created_at, updated_at FROM leads WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var l domain.Lead
	var email, phone sql.NullString
	var stateStr string
	var visitScheduledAt sql.NullTime

	err := row.Scan(&l.ID, &l.PropertyID, &l.ClientName, &email, &phone, &stateStr, &visitScheduledAt, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al buscar lead", err)
	}

	l.Email = email.String
	l.Phone = phone.String
	l.State = domain.LeadState(stateStr)
	if visitScheduledAt.Valid {
		t := visitScheduledAt.Time
		l.VisitScheduledAt = &t
	}

	return &l, nil
}

func nullTimeFromPtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
