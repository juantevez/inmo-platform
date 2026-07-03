package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"inmo.platform/contexts/maintenance/internal/domain"
)

type PostgresTicketRepository struct {
	db *sql.DB
}

func NewPostgresTicketRepository(db *sql.DB) *PostgresTicketRepository {
	return &PostgresTicketRepository{db: db}
}

// InitDB helper para reusar tu conexión Alpine de Docker
func InitDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// Save realiza un Upsert completo del Agregado de Mantenimiento
func (r *PostgresTicketRepository) Save(ctx context.Context, t *domain.Ticket) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Manejo de tipos Nulos/Opcionales para la consulta SQL
	var providerID, quoteDetails, evidenceDesc, evidenceURL sql.NullString
	var quoteAmount sql.NullFloat64
	var quoteAt, closedAt sql.NullTime

	if t.ProviderID != "" {
		providerID = sql.NullString{String: t.ProviderID, Valid: true}
	}
	if t.Quote != nil {
		quoteAmount = sql.NullFloat64{Float64: t.Quote.Amount, Valid: true}
		quoteDetails = sql.NullString{String: t.Quote.Details, Valid: true}
		quoteAt = sql.NullTime{Time: t.Quote.QuotedAt, Valid: true}
	}
	if t.Evidence != nil {
		evidenceDesc = sql.NullString{String: t.Evidence.Description, Valid: true}
		evidenceURL = sql.NullString{String: t.Evidence.DocumentURL, Valid: true}
		closedAt = sql.NullTime{Time: t.Evidence.ClosedAt, Valid: true}
	}

	// 2. Query de Upsert relacional
	query := `
		INSERT INTO tickets (
			id, property_id, tenant_id, provider_id, description, status, urgency,
			quote_amount, quote_details, quote_at, evidence_description, evidence_url, closed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			provider_id = EXCLUDED.provider_id,
			status = EXCLUDED.status,
			quote_amount = EXCLUDED.quote_amount,
			quote_details = EXCLUDED.quote_details,
			quote_at = EXCLUDED.quote_at,
			evidence_description = EXCLUDED.evidence_description,
			evidence_url = EXCLUDED.evidence_url,
			closed_at = EXCLUDED.closed_at;
	`

	_, err = tx.ExecContext(ctx, query,
		t.ID, t.PropertyID, t.TenantID, providerID, t.Description, string(t.Status), string(t.Urgency),
		quoteAmount, quoteDetails, quoteAt, evidenceDesc, evidenceURL, closedAt, t.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// FindByID reconstruye el Agregado hidratando sus componentes desde Postgres
func (r *PostgresTicketRepository) FindByID(ctx context.Context, id string) (*domain.Ticket, error) {
	query := `
		SELECT 
			id, property_id, tenant_id, provider_id, description, status, urgency,
			quote_amount, quote_details, quote_at, evidence_description, evidence_url, closed_at, created_at
		FROM tickets WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)

	var t domain.Ticket
	var statusStr, urgencyStr string
	var providerID, quoteDetails, evidenceDesc, evidenceURL sql.NullString
	var quoteAmount sql.NullFloat64
	var quoteAt, closedAt sql.NullTime

	err := row.Scan(
		&t.ID, &t.PropertyID, &t.TenantID, &providerID, &t.Description, &statusStr, &urgencyStr,
		&quoteAmount, &quoteDetails, &quoteAt, &evidenceDesc, &evidenceURL, &closedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// Rehidratar enums y opcionales al Agregado Rico
	t.Status = domain.TicketStatus(statusStr)
	t.Urgency = domain.UrgencyLevel(urgencyStr)

	if providerID.Valid {
		t.ProviderID = providerID.String
	}
	if quoteAmount.Valid {
		t.Quote = &domain.Quote{
			Amount:   quoteAmount.Float64,
			Details:  quoteDetails.String,
			QuotedAt: quoteAt.Time,
		}
	}
	if evidenceDesc.Valid {
		t.Evidence = &domain.Evidence{
			Description: evidenceDesc.String,
			DocumentURL: evidenceURL.String,
			ClosedAt:    closedAt.Time,
		}
	}

	return &t, nil
}
