package postgres

import (
	"context"
	"database/sql"
)

type PostgresOutboxRepository struct {
	// Podemos dejar el db *sql.DB por si en algún momento necesitamos operaciones fuera de Tx
	db *sql.DB
}

func NewPostgresOutboxRepository(db *sql.DB) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

// SaveTx inserta el evento usando la transacción provista por la capa de aplicación/repositorio raíz
func (r *PostgresOutboxRepository) SaveTx(ctx context.Context, tx *sql.Tx, eventName string, payload []byte) error {
	query := `
		INSERT INTO finances_outbox_events (event_name, payload, status)
		VALUES ($1, $2, 'PENDING');
	`
	_, err := tx.ExecContext(ctx, query, eventName, payload)
	return err
}
