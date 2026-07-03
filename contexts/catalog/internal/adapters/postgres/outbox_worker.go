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

// NewOutboxWorker ahora inicializa internamente el EventPublisher compartido usando la instancia de JetStream
func NewOutboxWorker(db *sql.DB, js jetstream.JetStream) *OutboxWorker {
	return &OutboxWorker{
		db:        db,
		publisher: eventbus.NewEventPublisher(js),
	}
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
	// 1. Iniciamos una transacción para asegurar consistencia al leer y bloquear las filas
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 2. Buscamos los eventos pendientes aislándolos con SKIP LOCKED
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
		return nil // Nada para procesar
	}

	log.Printf("[OUTBOX WORKER] Encontrados %d eventos pendientes. Despachando mediante infraestructura compartida...\n", len(pendingEvents))

	// 3. Publicamos cada evento usando el EventPublisher centralizado
	for _, event := range pendingEvents {
		// Despacha a NATS controlando de forma segura sus propios timeouts internos
		err := w.publisher.Publish(ctx, event.eventName, event.payload)
		if err != nil {
			log.Printf("[OUTBOX WARN] No se pudo enviar el evento %s a NATS. Se reintentará en la próxima pasada. Err: %v\n", event.id, err)
			return err
		}

		// 4. Si el publicador dio el OK (recibió PubAck de NATS), actualizamos la base de datos
		updateQuery := `UPDATE outbox_events SET status = 'PROCESSED', processed_at = CURRENT_TIMESTAMP WHERE id = $1;`
		_, err = tx.ExecContext(ctx, updateQuery, event.id)
		if err != nil {
			return err
		}
		log.Printf("[OUTBOX WORKER] Evento %s marcado como PROCESSED exitosamente.\n", event.id)
	}

	// 5. Confirmamos la Tx en Postgres de manera atómica
	return tx.Commit()
}
