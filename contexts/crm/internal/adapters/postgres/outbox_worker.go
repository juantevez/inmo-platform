package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"inmo.platform/shared/pkg/eventbus"
)

type OutboxWorker struct {
	db        *sql.DB
	publisher *eventbus.EventPublisher
}

func NewOutboxWorker(db *sql.DB, js jetstream.JetStream) *OutboxWorker {
	return &OutboxWorker{
		db:        db,
		publisher: eventbus.NewEventPublisher(js),
	}
}

func (w *OutboxWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[CRM OUTBOX] Worker iniciado. Escaneando cada %v...\n", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[CRM OUTBOX] Deteniendo worker de CRM...")
			return
		case <-ticker.C:
			if err := w.processEvents(ctx); err != nil {
				log.Printf("[CRM OUTBOX ERROR] %v\n", err)
			}
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Importante: Usamos una tabla específica crm_outbox_events para no colisionar
	// si compartimos la base de datos inmo_catalog_db
	query := `
        SELECT id, event_name, payload 
        FROM crm_outbox_events 
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

	type row struct {
		id      string
		subject string
		payload []byte
	}

	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.subject, &r.payload); err != nil {
			return err
		}
		pending = append(pending, r)
	}

	if len(pending) == 0 {
		return nil
	}

	log.Printf("[CRM OUTBOX] Despachando %d eventos a NATS...\n", len(pending))

	for _, ev := range pending {
		if err := w.publisher.Publish(ctx, ev.subject, ev.payload); err != nil {
			log.Printf("[CRM OUTBOX WARN] Error publicando %s: %v\n", ev.id, err)
			return err
		}

		updateQuery := `UPDATE crm_outbox_events SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP WHERE id = $1;`
		if _, err := tx.ExecContext(ctx, updateQuery, ev.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
