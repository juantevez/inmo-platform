package ports

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
)

// TicketRepository define las operaciones de persistencia del Agregado Ticket
type TicketRepository interface {
	Save(ctx context.Context, ticket *domain.Ticket) error
	FindByID(ctx context.Context, id string) (*domain.Ticket, error)
}

// ProviderRepository define las operaciones de persistencia del Agregado Provider.
type ProviderRepository interface {
	Save(ctx context.Context, provider *domain.Provider) error
	Update(ctx context.Context, provider *domain.Provider) error
	FindByID(ctx context.Context, id string) (*domain.Provider, error)
	FindByUserID(ctx context.Context, userID string) (*domain.Provider, error)
	FindByCuitCuil(ctx context.Context, cuitCuil string) (*domain.Provider, error)
	FindByRubro(ctx context.Context, rubro domain.RubroTecnico) ([]*domain.Provider, error)
	FindAvailableForEmergency(ctx context.Context, rubro domain.RubroTecnico) ([]*domain.Provider, error)
}

// PropertyProjectionRepository gestiona la tabla espejo de propiedades en maintenance_db.
//
// Esta proyección se mantiene sincronizada vía eventos NATS de catalog.
// No hay FK física entre property_projections y las tablas de catalog —
// la relación es lógica a nivel de aplicación.
type PropertyProjectionRepository interface {
	// Upsert inserta o actualiza la proyección cuando llega un evento de catalog.
	// Si la propiedad ya existe (re-publicación), actualiza todos los campos.
	Upsert(ctx context.Context, projection *domain.PropertyProjection) error

	// FindByID recupera la proyección local para validar si existe la propiedad
	// antes de crear un ticket.
	FindByID(ctx context.Context, propertyID string) (*domain.PropertyProjection, error)

	// UpdateState actualiza solo el campo state cuando llega catalog.property.state_changed.
	// Más eficiente que un Upsert completo para cambios de estado frecuentes.
	UpdateState(ctx context.Context, propertyID, newState string) error
}
