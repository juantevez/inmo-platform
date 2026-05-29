package inmemory

import (
	"context"
	"fmt"
	"inmo.platform/shared/pkg/ddd"
)

type EventPublisher struct{}

func NewEventPublisher() *EventPublisher {
	return &EventPublisher{}
}

func (p *EventPublisher) Publish(ctx context.Context, events ...ddd.DomainEvent) error {
	for _, event := range events {
		// Por ahora simulamos la salida por consola.
		// Cuando conectemos NATS, esto enviará el JSON al stream correspondiente.
		fmt.Printf("[NATS MOCK PUBLISH] Evento emitido: %s | Agregado ID: %s\n", event.EventName(), event.AggregateID())
	}
	return nil
}
