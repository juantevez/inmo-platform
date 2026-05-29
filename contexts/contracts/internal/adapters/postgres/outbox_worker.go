package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type OutboxEventRow struct {
	ID        string
	EventName string
	Payload   []byte
}

type OutboxWorker struct {
	db *sql.DB
	js jetstream.JetStream
}

func NewOutboxWorker(db *sql.DB, js jetstream.JetStream) *OutboxWorker {
	return &OutboxWorker{db: db, js: js}
}

func (w *OutboxWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[CONTRACTS OUTBOX] Worker iniciado. Escaneando cada %v...\n", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processEvents(ctx); err != nil {
				log.Printf("[CONTRACTS OUTBOX ERROR] %v\n", err)
			}
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) error {
	// Iniciamos transacción para el vaciado seguro
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// FOR UPDATE SKIP LOCKED evita que múltiples réplicas pisen el mismo evento
	query := `
		SELECT id, event_name, payload 
		FROM contracts_outbox_events 
		WHERE status = 'PENDING' 
		ORDER BY created_at ASC 
		LIMIT 10 
		FOR UPDATE SKIP LOCKED;
	`

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var events []OutboxEventRow
	for rows.Next() {
		var ev OutboxEventRow
		if err := rows.Scan(&ev.ID, &ev.EventName, &ev.Payload); err != nil {
			return err
		}
		events = append(events)
	}

	if len(events) == 0 {
		return nil
	}

	log.Printf("[CONTRACTS OUTBOX] Encontrados %d eventos pendientes. Despachando a NATS...\n", len(events))

	for _, ev := range events {
		// Publicamos dinámicamente en NATS JetStream usando el nombre del evento como Subject
		// El evento será: contracts.contract.activated
		_, err := w.js.Publish(ctx, ev.EventName, ev.Payload)
		if err != nil {
			log.Printf("[CONTRACTS OUTBOX WARN] No se pudo publicar %s: %v. Se reintentará...\n", ev.ID, err)
			continue
		}

		// Marcamos como procesado
		updateQuery := `
			UPDATE contracts_outbox_events 
			SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP 
			WHERE id = $1;
		`
		if _, err := tx.ExecContext(ctx, updateQuery, ev.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}
