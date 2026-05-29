package application

import (
	"context"
	"fmt"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
)

type CreateAutoLeadDTO struct {
	PropertyID string
	OwnerID    string
}

type CreateAutoLeadUseCase struct {
	repo ports.LeadRepository
}

func NewCreateAutoLeadUseCase(repo ports.LeadRepository) *CreateAutoLeadUseCase {
	return &CreateAutoLeadUseCase{repo: repo}
}

func (uc *CreateAutoLeadUseCase) Execute(ctx context.Context, dto CreateAutoLeadDTO) error {
	// Generamos un ID interno único para el Lead de control
	leadID := fmt.Sprintf("lead-auto-%s", dto.PropertyID)

	// Creamos el lead asociándolo a la propiedad que nos llegó del evento
	lead, err := domain.NewLead(
		leadID,
		dto.PropertyID,
		"SISTEMA - CAPTACION AUTOMATICA",
		"sistema@inmo-platform.com",
		"0800-INMO",
	)
	if err != nil {
		return err
	}

	// Persistimos el lead en el almacenamiento de CRM
	if err := uc.repo.Save(ctx, lead); err != nil {
		return err
	}

	return nil
}
