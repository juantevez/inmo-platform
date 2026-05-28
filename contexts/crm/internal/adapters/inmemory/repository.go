package inmemory

import (
	"context"
	"inmo-platform/contexts/crm/internal/domain"
	"log"
	"sync"
)

type LeadRepository struct {
	mu    sync.RWMutex
	leads map[string]*domain.Lead
}

func NewLeadRepository() *LeadRepository {
	return &LeadRepository{
		leads: make(map[string]*domain.Lead),
	}
}

func (r *LeadRepository) Save(ctx context.Context, lead *domain.Lead) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leads[lead.ID()] = lead
	log.Printf("[CRM DB MOCK] Lead guardado exitosamente -> ID: %s | Estado: %s | Propiedad vinculada: %s\n", lead.ID(), lead.State(), lead.PropertyID())
	return nil
}
