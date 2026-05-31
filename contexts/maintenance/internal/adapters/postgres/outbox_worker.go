package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type OutboxEvent struct {
	ID        int64
	EventName string
	Payload   []byte
}

type OutboxWorker struct {
	db *sql.DB
	js jetstream.JetStream
}

func NewOutboxWorker(db *sql.DB, js jetstream.JetStream) *OutboxWorker {
	return &OutboxWorker{
		db: db,
		js: js,
	}
}

// Start inicia el bucle infinito de escaneo en segundo plano
func (w *OutboxWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[MAINTENANCE OUTBOX] Deteniendo el worker...")
			return
		case <-ticker.C:
			if err := w.processEvents(ctx); err != nil {
				log.Printf("[MAINTENANCE OUTBOX] Error procesando eventos: %v", err)
			}
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) error {
	// 1. Buscamos los eventos PENDING
	query := `
		SELECT id, event_name, payload 
		FROM maintenance_outbox_events 
		WHERE status = 'PENDING' 
		ORDER BY created_at ASC 
		LIMIT 10;
	`
	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var ev OutboxEvent
		if err := rows.Scan(&ev.ID, &ev.EventName, &ev.Payload); err != nil {
			return err
		}
		events = append(events, ev)
	}

	// 2. Despachamos cada uno a NATS
	for _, ev := range events {
		// Publicamos de forma asincrónica en JetStream usando el EventName como Subject
		_, err := w.js.Publish(ctx, ev.EventName, ev.Payload)
		if err != nil {
			log.Printf("[MAINTENANCE OUTBOX] Error al publicar evento %d en NATS: %v", ev.ID, err)
			continue // Si falla NATS, queda PENDING para el próximo ciclo
		}

		// 3. Si NATS aceptó el evento, lo marcamos como PROCESSED
		updateQuery := `
			UPDATE maintenance_outbox_events 
			SET status = 'PROCESSED', processed_at = NOW() 
			WHERE id = $1;
		`
		_, err = w.db.ExecContext(ctx, updateQuery, ev.ID)
		if err != nil {
			log.Printf("[MAINTENANCE OUTBOX] Error al actualizar estado del evento %d: %v", ev.ID, err)
		}
	}

	return nil
}
