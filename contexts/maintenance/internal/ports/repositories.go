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
type PropertyProjectionRepository interface {
	Upsert(ctx context.Context, projection *domain.PropertyProjection) error
	FindByID(ctx context.Context, propertyID string) (*domain.PropertyProjection, error)
	UpdateState(ctx context.Context, propertyID, newState string) error
}

// InquilinoProjectionRepository gestiona la tabla espejo de inquilinos en maintenance_db.
//
// Se sincroniza via el evento auth.user.created (role = INQUILINO).
// Permite que maintenance valide si quien abre un ticket es un inquilino activo
// sin consultar auth_db en cada operación.
type InquilinoProjectionRepository interface {
	// Upsert inserta o actualiza la proyección cuando llega auth.user.created.
	// Si el inquilino ya existe (reenvío del evento), actualiza email y synced_at.
	Upsert(ctx context.Context, projection *domain.InquilinoProjection) error

	// FindByUserID recupera la proyección por user_id.
	// Retorna nil, nil si no existe — el inquilino no está registrado aún.
	FindByUserID(ctx context.Context, userID string) (*domain.InquilinoProjection, error)

	// UpdateStatus actualiza el status cuando llega auth.user.suspended o auth.user.activated.
	UpdateStatus(ctx context.Context, userID, newStatus string) error
}
