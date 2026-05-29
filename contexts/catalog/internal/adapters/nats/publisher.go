package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
	"log"

	"github.com/nats-io/nats.go/jetstream"
)

type EventPublisher struct {
	js jetstream.JetStream
}

func NewEventPublisher(js jetstream.JetStream) *EventPublisher {
	return &EventPublisher{js: js}
}

func (p *EventPublisher) Publish(ctx context.Context, events ...ddd.DomainEvent) error {
	for _, event := range events {
		// 1. Serializar el evento a JSON
		payload, err := json.Marshal(event)
		if err != nil {
			return apperr.NewInternal("error al serializar evento de dominio a json", err)
		}

		// 2. Publicar en JetStream usando su EventName como Subject jerárquico
		subject := event.EventName() // ej: "catalog.property.published"

		_, err = p.js.Publish(ctx, subject, payload)
		if err != nil {
			return apperr.NewInternal(fmt.Sprintf("error al enviar mensaje al subject %s", subject), err)
		}

		log.Printf("[NATS JETSTREAM] Mensaje enviado a JetStream -> Subject: %s | ID: %s\n", subject, event.AggregateID())
	}
	return nil
}
