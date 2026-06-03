package ports

import (
	"context"

	"inmo.platform/contexts/crm/internal/domain"
)

type LeadRepository interface {
	// Save guarda o actualiza un lead y enlista su evento outbox de forma transaccional
	Save(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error
	GetByID(ctx context.Context, id string) (*domain.Lead, error)
}
