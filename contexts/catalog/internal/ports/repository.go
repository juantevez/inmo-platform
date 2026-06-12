package ports

import (
	"context"

	"inmo.platform/contexts/catalog/internal/domain"
)

// ListFilters define los criterios de filtrado y paginación para listar propiedades.
type ListFilters struct {
	State         string
	OperationType string
	PetPolicy     string
	OwnerID       string
	MinPrice      float64
	MaxPrice      float64
	Limit         int
	Offset        int
	// 🗺️ Búsqueda geoespacial — se activa solo si RadiusKm > 0
	Latitude  float64
	Longitude float64
	RadiusKm  float64
}

// PropertyResult envuelve el agregado con datos de infraestructura opcionales
// que el dominio no conoce (ej: distancia calculada por PostGIS).
type PropertyResult struct {
	Property  *domain.Property
	DistanceM *float64 // nil si la consulta no fue geoespacial
}

// PropertyRepository define el contrato para persistir y recuperar el agregado Property.
type PropertyRepository interface {
	Save(ctx context.Context, property *domain.Property) error
	FindByID(ctx context.Context, id string) (*domain.Property, error)
	// FindAll devuelve la página de resultados y el total antes de paginar.
	FindAll(ctx context.Context, filters ListFilters) ([]PropertyResult, int, error)
}
