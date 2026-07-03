package postgres

import (
	"context"
	"database/sql" // 🚀 Volvemos a usar el estándar
	"fmt"
	"time"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
)

type PostgresLeadRepository struct {
	db *sql.DB // 🚀 Cambiado a *sql.DB
}

func NewPostgresLeadRepository(db *sql.DB) ports.LeadRepository { // 🚀 Cambiado a *sql.DB
	return &PostgresLeadRepository{db: db}
}

func (r *PostgresLeadRepository) Save(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
	// Iniciamos la transacción estándar
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error al iniciar transaccion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Insertar o Actualizar el Lead (Upsert)
	leadQuery := `
        INSERT INTO leads (id, property_id, client_name, email, phone, state, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (id) DO UPDATE SET
            client_name = EXCLUDED.client_name,
            email = EXCLUDED.email,
            phone = EXCLUDED.phone,
            state = EXCLUDED.state,
            updated_at = EXCLUDED.updated_at;
    `

	_, err = tx.ExecContext(ctx, leadQuery,
		lead.ID,
		lead.PropertyID,
		lead.ClientName,
		sql.NullString{String: lead.Email, Valid: lead.Email != ""}, // sql.NullString funciona nativo acá
		sql.NullString{String: lead.Phone, Valid: lead.Phone != ""},
		string(lead.State),
		lead.CreatedAt,
		lead.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("error al persistir el lead: %w", err)
	}

	// 2. Outbox
	if eventName != "" && len(eventPayload) > 0 {
		outboxQuery := `
            INSERT INTO crm_outbox_events (event_name, payload, status, created_at)
            VALUES ($1, $2, 'PENDING', $3);
        `
		_, err = tx.ExecContext(ctx, outboxQuery, eventName, eventPayload, time.Now())
		if err != nil {
			return fmt.Errorf("error al enlistar evento outbox: %w", err)
		}
	}

	return tx.Commit()
}

func (r *PostgresLeadRepository) GetByID(ctx context.Context, id string) (*domain.Lead, error) {
	query := `SELECT id, property_id, client_name, email, phone, state, created_at, updated_at FROM leads WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)

	var l domain.Lead
	var email, phone sql.NullString
	var stateStr string

	err := row.Scan(&l.ID, &l.PropertyID, &l.ClientName, &email, &phone, &stateStr, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("error al buscar lead: %w", err)
	}

	l.Email = email.String
	l.Phone = phone.String
	l.State = domain.LeadState(stateStr)

	return &l, nil
}
