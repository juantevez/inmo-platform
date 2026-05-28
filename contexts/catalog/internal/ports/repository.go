package ports

import (
	"context"

	"inmo-platform/contexts/catalog/internal/domain"
)

// PropertyRepository define el contrato para persistir y recuperar el agregado Property.
type PropertyRepository interface {
	Save(ctx context.Context, property *domain.Property) error
	FindByID(ctx context.Context, id string) (*domain.Property, error)
}
