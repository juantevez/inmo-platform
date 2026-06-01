package application

import (
	"context"
	"time"

	"inmo.platform/contexts/catalog/internal/ports"
)

type ListPropertyMediaUseCase struct {
	mediaRepo ports.MediaRepository
}

func NewListPropertyMediaUseCase(mediaRepo ports.MediaRepository) *ListPropertyMediaUseCase {
	return &ListPropertyMediaUseCase{mediaRepo: mediaRepo}
}

type MediaDTO struct {
	ID          string            `json:"id"`
	PropertyID  string            `json:"property_id"`
	URL         string            `json:"url,omitempty"`
	Type        string            `json:"type"`
	SortOrder   int               `json:"sort_order"`
	SocialLinks map[string]string `json:"social_links,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

func (uc *ListPropertyMediaUseCase) Execute(ctx context.Context, propertyID string) ([]MediaDTO, error) {
	items, err := uc.mediaRepo.FindByPropertyID(ctx, propertyID)
	if err != nil {
		return nil, err
	}

	dtos := make([]MediaDTO, 0, len(items))
	for _, m := range items {
		dtos = append(dtos, MediaDTO{
			ID:          m.ID(),
			PropertyID:  m.PropertyID(),
			URL:         m.URL(),
			Type:        string(m.Type()),
			SortOrder:   m.SortOrder(),
			SocialLinks: m.SocialLinks(),
			CreatedAt:   m.CreatedAt(),
		})
	}
	return dtos, nil
}
