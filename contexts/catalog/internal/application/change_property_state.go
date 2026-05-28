package application

import (
	"context"
	"inmo-platform/contexts/catalog/internal/ports"
	"inmo-platform/shared/pkg/apperr"
)

type ChangeStateAction string

const (
	ActionReserve       ChangeStateAction = "RESERVE"
	ActionClose         ChangeStateAction = "CLOSE"
	ActionUnderRepair   ChangeStateAction = "PUT_UNDER_REPAIR"
	ActionReleaseRepair ChangeStateAction = "RELEASE_REPAIR"
)

type ChangePropertyStateUseCase struct {
	repo      ports.PropertyRepository
	publisher ports.EventPublisher
}

func NewChangePropertyStateUseCase(repo ports.PropertyRepository, publisher ports.EventPublisher) *ChangePropertyStateUseCase {
	return &ChangePropertyStateUseCase{
		repo:      repo,
		publisher: publisher,
	}
}

func (uc *ChangePropertyStateUseCase) Execute(ctx context.Context, propertyID string, action ChangeStateAction) error {
	// 1. Recuperar el Agregado Raíz desde el puerto
	property, err := uc.repo.FindByID(ctx, propertyID)
	if err != nil {
		return err
	}
	if property == nil {
		return apperr.NewNotFound("no se encontró la propiedad especificada", nil)
	}

	// 2. Ejecutar la mutación de estado a través del Dominio (donde se validan las invariantes)
	switch action {
	case ActionReserve:
		err = property.Reserve()
	case ActionClose:
		err = property.Close()
	case ActionUnderRepair:
		err = property.PutUnderRepair()
	case ActionReleaseRepair:
		err = property.ReleaseRepair()
	default:
		return apperr.NewBadRequest("acción de cambio de estado no reconocida", nil)
	}

	if err != nil {
		return err
	}

	// 3. Persistir el nuevo estado de la entidad
	if err := uc.repo.Save(ctx, property); err != nil {
		return err
	}

	// 4. Publicar los eventos resultantes de la transición de estado
	events := property.PullEvents()
	if len(events) > 0 {
		_ = uc.publisher.Publish(ctx, events...)
	}

	return nil
}
