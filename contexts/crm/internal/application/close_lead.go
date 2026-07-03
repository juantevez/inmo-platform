package application

import (
	"context"
	"errors"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// CloseLeadUseCase cierra el lead (abandono o finalización del funnel).
type CloseLeadUseCase struct {
	repo ports.LeadRepository
}

func NewCloseLeadUseCase(repo ports.LeadRepository) *CloseLeadUseCase {
	return &CloseLeadUseCase{repo: repo}
}

func (uc *CloseLeadUseCase) Execute(ctx context.Context, leadID string) (*LeadDTO, error) {
	lead, err := uc.repo.GetByID(ctx, leadID)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, apperr.NewNotFound("lead no encontrado", nil)
	}

	if err := lead.Close(); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil, apperr.NewPreconditionFailed(err.Error(), err)
		}
		return nil, err
	}

	// "Cerrado" tampoco es un evento de integración definido para CRM — se
	// persiste el estado sin encolar ningún evento nuevo en el outbox.
	if err := uc.repo.Save(ctx, lead, "", nil); err != nil {
		return nil, err
	}

	return toLeadDTO(lead), nil
}
