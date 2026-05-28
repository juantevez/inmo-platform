package ports

import (
	"context"
	"inmo-platform/contexts/crm/internal/domain"
)

type LeadRepository interface {
	Save(ctx context.Context, lead *domain.Lead) error
}
