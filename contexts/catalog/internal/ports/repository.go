package ports

import (
	"context"

	"inmo.platform/contexts/catalog/internal/domain"
)

// ListFilters define los criterios de filtrado y paginación para listar propiedades.
type ListFilters struct {
	State    string
	MinPrice float64
	MaxPrice float64
	Limit    int
	Offset   int
}

// PropertyRepository define el contrato para persistir y recuperar el agregado Property.
type PropertyRepository interface {
	Save(ctx context.Context, property *domain.Property) error
	FindByID(ctx context.Context, id string) (*domain.Property, error)
	// FindAll devuelve la página de resultados y el total antes de paginar.
	FindAll(ctx context.Context, filters ListFilters) ([]*domain.Property, int, error)
}
