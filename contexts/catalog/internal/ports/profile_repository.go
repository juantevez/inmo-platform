package ports

import (
	"context"

	"inmo.platform/contexts/catalog/internal/domain"
)

type ProfileRepository interface {
	Save(ctx context.Context, profile *domain.Profile) error
	FindByID(ctx context.Context, userID string) (*domain.Profile, error)
	FindByDniCuit(ctx context.Context, dniCuit string) (*domain.Profile, error)
}
