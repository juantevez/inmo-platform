package ports

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
)

// EventDispatcher publica eventos de integración hacia el broker de mensajería
type EventDispatcher interface {
	DispatchApproved(ctx context.Context, event domain.TicketApprovedEvent) error
	DispatchClosed(ctx context.Context, event domain.TicketClosedEvent) error
}

// CatalogService es el puerto para consultar datos de catalog que NO están en la proyección local.
//
// Opción B: en lugar de almacenar todo en la proyección, consultamos catalog
// via NATS request/reply solo cuando el dato es necesario en tiempo real.
//
// Actualmente cubre:
//   - PropertyExists: validación rápida usando la proyección local (sin ir a catalog)
//   - GetPropertyLocation: consulta la dirección/coordenadas al asignar la orden al técnico
//
// La implementación real usa NATS request/reply con subject "catalog.property.query.location".
// El stub simula la respuesta para desarrollo local.
type CatalogService interface {
	// PropertyExists verifica si la propiedad tiene proyección local en maintenance.
	// Reemplaza la consulta directa a catalog — si no hay proyección, el evento
	// catalog.property.published todavía no llegó (lag de NATS) o la propiedad no existe.
	PropertyExists(ctx context.Context, propertyID string) (bool, error)

	// GetPropertyLocation consulta la dirección y coordenadas de una propiedad a catalog
	// via NATS request/reply. Solo se llama cuando el técnico necesita la dirección,
	// no en cada operación de ticket.
	GetPropertyLocation(ctx context.Context, propertyID string) (*PropertyLocationResult, error)
}

// PropertyLocationResult contiene los datos de ubicación devueltos por catalog.
type PropertyLocationResult struct {
	PropertyID string  `json:"property_id"`
	Address    string  `json:"address"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Title      string  `json:"title"`
}
