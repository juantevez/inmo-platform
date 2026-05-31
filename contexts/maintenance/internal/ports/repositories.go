package ports

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
)

// TicketRepository define las operaciones de persistencia del Agregado Ticket
type TicketRepository interface {
	// Save inserta un nuevo ticket o actualiza su estado completo (Upsert transaccional)
	Save(ctx context.Context, ticket *domain.Ticket) error

	// FindByID recupera un ticket por su identificador único para aplicar mutaciones
	FindByID(ctx context.Context, id string) (*domain.Ticket, error)
}
