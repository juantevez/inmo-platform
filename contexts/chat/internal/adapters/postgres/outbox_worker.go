package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"inmo.platform/shared/pkg/eventbus"
)

type ChatOutboxWorker struct {
	db        *sql.DB
	publisher *eventbus.EventPublisher
}

func NewChatOutboxWorker(db *sql.DB, js jetstream.JetStream) *ChatOutboxWorker {
	return &ChatOutboxWorker{
		db:        db,
		publisher: eventbus.NewEventPublisher(js),
	}
}

func (w *ChatOutboxWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[CHAT OUTBOX] Worker iniciado. Escaneando cada %v...\n", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[CHAT OUTBOX] Deteniendo worker de forma ordenada...")
			return
		case <-ticker.C:
			if err := w.processEvents(ctx); err != nil {
				log.Printf("[CHAT OUTBOX ERROR] %v\n", err)
			}
		}
	}
}

func (w *ChatOutboxWorker) processEvents(ctx context.Context) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, subject, payload
		FROM chat_outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT 10
		FOR UPDATE SKIP LOCKED`)
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
	_ = rows.Close()

	if len(pending) == 0 {
		return nil
	}

	log.Printf("[CHAT OUTBOX] Despachando %d eventos a NATS...\n", len(pending))

	for _, ev := range pending {
		if err := w.publisher.Publish(ctx, ev.subject, ev.payload); err != nil {
			log.Printf("[CHAT OUTBOX WARN] Error publicando %s: %v\n", ev.id, err)
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE chat_outbox_events
			SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP
			WHERE id = $1`, ev.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
