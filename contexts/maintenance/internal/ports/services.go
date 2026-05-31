package ports

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
)

// EventDispatcher se encarga de publicar los eventos de integración hacia el broker de mensajería
type EventDispatcher interface {
	// DispatchApproved publica el evento de ticket aprobado (Catálogo escuchará esto)
	DispatchApproved(ctx context.Context, event domain.TicketApprovedEvent) error

	// DispatchClosed publica el evento de cierre de ticket (Catálogo liberará la propiedad)
	DispatchClosed(ctx context.Context, event domain.TicketClosedEvent) error
}

// CatalogService es un puerto conductor para validar datos del dominio de Catálogo (BFF o RPC)
type CatalogService interface {
	// PropertyExists verifica si el ID de la propiedad es real y está activo para mantenimiento
	PropertyExists(ctx context.Context, propertyID string) (bool, error)
}
