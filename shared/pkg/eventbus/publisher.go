package eventbus

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// EventPublisher abstrae la publicación de mensajes hacia NATS JetStream.
type EventPublisher struct {
	js jetstream.JetStream
}

// NewEventPublisher crea una nueva instancia del publicador genérico.
func NewEventPublisher(js jetstream.JetStream) *EventPublisher {
	return &EventPublisher{js: js}
}

// Publish toma un subject y un payload de bytes del Outbox y los empuja a NATS.
// Retorna error si el broker no confirma el almacenamiento del mensaje (PubAck).
func (p *EventPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	log.Printf("[NATS PUBLISHER] Intentando despachar evento | Subject: %s | Tamaño: %d bytes\n", subject, len(payload))

	// Ponemos un timeout estricto de 3 segundos para no congelar el loop del Outbox Worker
	pubCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Publicación con ACK nativo de JetStream
	pubAck, err := p.js.Publish(pubCtx, subject, payload)
	if err != nil {
		return fmt.Errorf("error de publicación en JetStream (subject: %s): %w", subject, err)
	}

	log.Printf("[NATS PUBLISHER SUCCESS] Mensaje persistido por NATS | Stream: %s | Secuencia: %d\n", pubAck.Stream, pubAck.Sequence)
	return nil
}
