package inmemory

import (
	"context"
	"sync"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
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

func (r *PropertyRepository) FindAll(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*domain.Property
	for _, p := range r.properties {
		if f.State != "" && string(p.State()) != f.State {
			continue
		}
		if f.MinPrice > 0 && p.Price().Amount() < f.MinPrice {
			continue
		}
		if f.MaxPrice > 0 && p.Price().Amount() > f.MaxPrice {
			continue
		}
		filtered = append(filtered, p)
	}

	total := len(filtered)

	if f.Offset > 0 {
		if f.Offset >= len(filtered) {
			return []ports.PropertyResult{}, total, nil
		}
		filtered = filtered[f.Offset:]
	}
	if f.Limit > 0 && len(filtered) > f.Limit {
		filtered = filtered[:f.Limit]
	}

	results := make([]ports.PropertyResult, len(filtered))
	for i, p := range filtered {
		results[i] = ports.PropertyResult{Property: p, DistanceM: nil}
	}
	return results, total, nil
}
