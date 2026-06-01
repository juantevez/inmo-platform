package application

import (
	"context"

	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type GenerateUploadURLUseCase struct {
	propertyRepo ports.PropertyRepository
	storage      ports.MediaStorageProvider // nil when AWS is not configured
}

func NewGenerateUploadURLUseCase(propertyRepo ports.PropertyRepository, storage ports.MediaStorageProvider) *GenerateUploadURLUseCase {
	return &GenerateUploadURLUseCase{propertyRepo: propertyRepo, storage: storage}
}

type GenerateUploadURLCommand struct {
	PropertyID  string
	Filename    string
	ContentType string
	RequesterID string
}

type UploadURLResponse struct {
	PresignedURL string `json:"presigned_url"`
	FinalURL     string `json:"final_url"`
}

func (uc *GenerateUploadURLUseCase) Execute(ctx context.Context, cmd GenerateUploadURLCommand) (*UploadURLResponse, error) {
	if uc.storage == nil {
		return nil, apperr.NewBadRequest("almacenamiento en la nube no configurado (AWS_BUCKET_NAME no definido)", nil)
	}
	if cmd.PropertyID == "" || cmd.Filename == "" {
		return nil, apperr.NewBadRequest("property_id y filename son requeridos", nil)
	}

	property, err := uc.propertyRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return nil, err
	}
	if property == nil {
		return nil, apperr.NewNotFound("propiedad no encontrada", nil)
	}
	if property.OwnerID() != cmd.RequesterID {
		return nil, apperr.NewForbidden("solo el dueño puede subir archivos a esta propiedad", nil)
	}

	presignedURL, finalURL, err := uc.storage.GeneratePresignedURL(ctx, cmd.PropertyID, cmd.Filename, cmd.ContentType)
	if err != nil {
		return nil, err
	}

	return &UploadURLResponse{PresignedURL: presignedURL, FinalURL: finalURL}, nil
}
