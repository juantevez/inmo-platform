package application

import (
	"context"

	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type DeletePropertyMediaUseCase struct {
	propertyRepo ports.PropertyRepository
	mediaRepo    ports.MediaRepository
}

func NewDeletePropertyMediaUseCase(propertyRepo ports.PropertyRepository, mediaRepo ports.MediaRepository) *DeletePropertyMediaUseCase {
	return &DeletePropertyMediaUseCase{propertyRepo: propertyRepo, mediaRepo: mediaRepo}
}

type DeleteMediaCommand struct {
	PropertyID  string
	MediaID     string
	RequesterID string
}

func (uc *DeletePropertyMediaUseCase) Execute(ctx context.Context, cmd DeleteMediaCommand) error {
	property, err := uc.propertyRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return err
	}
	if property == nil {
		return apperr.NewNotFound("propiedad no encontrada", nil)
	}
	if property.OwnerID() != cmd.RequesterID {
		return apperr.NewForbidden("solo el dueño puede eliminar media de esta propiedad", nil)
	}
	return uc.mediaRepo.DeleteMedia(ctx, cmd.MediaID)
}
