package inmemory

import (
	"context"
	"inmo-platform/contexts/catalog/internal/domain"
	"sync"
)

type PropertyRepository struct {
	mu         sync.RWMutex
	properties map[string]*domain.Property
}

func NewPropertyRepository() *PropertyRepository {
	return &PropertyRepository{
		properties: make(map[string]*domain.Property),
	}
}

func (r *PropertyRepository) Save(ctx context.Context, property *domain.Property) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.properties[property.ID()] = property
	return nil
}

func (r *PropertyRepository) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prop, exists := r.properties[id]
	if !exists {
		return nil, nil
	}
	return prop, nil
}
