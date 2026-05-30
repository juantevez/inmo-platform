package postgres

import (
	"context"
	"database/sql"
	"fmt"
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
	// Log 1: Saber si el ticker efectivamente entra a la función
	log.Println("[CONTRACTS OUTBOX] Entrando a processEvents... Ejecutando Query...")

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error al iniciar tx: %w", err)
	}
	defer tx.Rollback()

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
		return fmt.Errorf("error en QueryContext: %w", err)
	}
	defer rows.Close()

	var events []OutboxEventRow
	for rows.Next() {
		var ev OutboxEventRow
		if err := rows.Scan(&ev.ID, &ev.EventName, &ev.Payload); err != nil {
			return fmt.Errorf("error en rows.Scan: %w", err)
		}
		events = append(events, ev)
	}

	// Log 2: Ver cuántos eventos leyó realmente del slice después del append
	log.Printf("[CONTRACTS OUTBOX] Cantidad de eventos mapeados en memoria: %d\n", len(events))

	if len(events) == 0 {
		return nil
	}

	for _, ev := range events {
		log.Printf("[CONTRACTS OUTBOX] Intentando publicar en NATS el evento: %s (ID: %s)\n", ev.EventName, ev.ID)

		_, err := w.js.Publish(ctx, ev.EventName, ev.Payload)
		if err != nil {
			// Log 3: Si NATS lo rechaza, acá salta el motivo exacto
			log.Printf("[CONTRACTS OUTBOX ERROR] NATS rechazó la publicación de %s: %v\n", ev.ID, err)
			continue
		}

		log.Printf("[CONTRACTS OUTBOX] Publicado con éxito en NATS. Actualizando estado a PROCESSED...\n")

		updateQuery := `UPDATE contracts_outbox_events SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP WHERE id = $1;`
		if _, err := tx.ExecContext(ctx, updateQuery, ev.ID); err != nil {
			return fmt.Errorf("error al actualizar status en bd: %w", err)
		}
	}

	return tx.Commit()
}
