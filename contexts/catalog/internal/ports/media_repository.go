package ports

import (
	"context"

	"inmo.platform/contexts/catalog/internal/domain"
)

type MediaRepository interface {
	SaveMedia(ctx context.Context, media *domain.PropertyMedia) error
	FindByPropertyID(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error)
	DeleteMedia(ctx context.Context, mediaID string) error
}
