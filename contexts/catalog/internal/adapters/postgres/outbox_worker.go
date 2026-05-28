package postgres

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type OutboxWorker struct {
	db *sql.DB
	js jetstream.JetStream
}

func NewOutboxWorker(db *sql.DB, js jetstream.JetStream) *OutboxWorker {
	return &OutboxWorker{db: db, js: js}
}

// Start inicia el loop en segundo plano. Se corta si el contexto principal del servidor muere.
func (w *OutboxWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[OUTBOX WORKER] Iniciado. Escaneando eventos pendientes cada %v...\n", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[OUTBOX WORKER] Deteniendo el worker de forma ordenada...")
			return
		case <-ticker.C:
			if err := w.processPendingEvents(ctx); err != nil {
				log.Printf("[OUTBOX ERROR] Error al procesar la bandeja de salida: %v\n", err)
			}
		}
	}
}

func (w *OutboxWorker) processPendingEvents(ctx context.Context) error {
	// 1. Iniciamos una transacción para asegurar consistencia
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 2. Buscamos los eventos pendientes. El truco "FOR UPDATE SKIP LOCKED" es vital en producción:
	// Permite que si tenés múltiples instancias de la API corriendo, no se pisen ni se bloqueen entre sí.
	query := `
		SELECT id, event_name, payload 
		FROM outbox_events 
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

	type outboxRow struct {
		id        string
		eventName string
		payload   []byte
	}

	var pendingEvents []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.id, &row.eventName, &row.payload); err != nil {
			return err
		}
		pendingEvents = append(pendingEvents, row)
	}

	if len(pendingEvents) == 0 {
		return nil // No hay nada que procesar en estos 20 segundos
	}

	log.Printf("[OUTBOX WORKER] Encontrados %d eventos pendientes. Despachando a NATS...\n", len(pendingEvents))

	// 3. Publicamos cada evento en NATS JetStream
	for _, event := range pendingEvents {
		_, err := w.js.Publish(ctx, event.eventName, event.payload)
		if err != nil {
			// Si falla NATS, salimos del loop. La Tx hace Rollback y los eventos se reintentan en los próximos 20 segundos.
			log.Printf("[OUTBOX WARN] No se pudo enviar el evento %s a NATS. Se reintentará. Err: %v\n", event.id, err)
			return err
		}

		// 4. Si NATS dio el OK, actualizamos el estado de la fila en la BD a PROCESSED
		updateQuery := `UPDATE outbox_events SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP WHERE id = $1;`
		_, err = tx.ExecContext(ctx, updateQuery, event.id)
		if err != nil {
			return err
		}
		log.Printf("[OUTBOX WORKER] Evento %s enviado a NATS y marcado como PROCESSED.\n", event.id)
	}

	// 5. Confirmamos la Tx en Postgres. Todo cambia de estado en un solo paso atómico.
	return tx.Commit()
}
