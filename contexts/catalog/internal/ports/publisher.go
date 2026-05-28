package ports

import (
	"context"
	"inmo-platform/shared/pkg/ddd"
)

// EventPublisher define el contrato para despachar eventos de dominio fuera del contexto.
type EventPublisher interface {
	Publish(ctx context.Context, events ...ddd.DomainEvent) error
}
