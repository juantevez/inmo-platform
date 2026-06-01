package application

import (
	"context"
	"fmt"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type AddPropertyMediaUseCase struct {
	propertyRepo ports.PropertyRepository
	mediaRepo    ports.MediaRepository
}

func NewAddPropertyMediaUseCase(propertyRepo ports.PropertyRepository, mediaRepo ports.MediaRepository) *AddPropertyMediaUseCase {
	return &AddPropertyMediaUseCase{propertyRepo: propertyRepo, mediaRepo: mediaRepo}
}

type AddMediaCommand struct {
	PropertyID  string
	URL         string
	Type        string
	SortOrder   int
	SocialLinks map[string]string
	RequesterID string
}

func (uc *AddPropertyMediaUseCase) Execute(ctx context.Context, cmd AddMediaCommand) error {
	property, err := uc.propertyRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return err
	}
	if property == nil {
		return apperr.NewNotFound("propiedad no encontrada", nil)
	}
	if property.OwnerID() != cmd.RequesterID {
		return apperr.NewForbidden("solo el dueño puede agregar media a esta propiedad", nil)
	}

	id := fmt.Sprintf("media-%d", time.Now().UnixNano())
	media, err := domain.NewPropertyMedia(id, cmd.PropertyID, cmd.URL, domain.MediaType(cmd.Type), cmd.SortOrder, cmd.SocialLinks)
	if err != nil {
		return err
	}

	return uc.mediaRepo.SaveMedia(ctx, media)
}
