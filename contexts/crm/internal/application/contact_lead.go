package application

import (
	"context"
	"errors"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// ContactLeadUseCase registra que un agente hizo el primer contacto con el lead.
type ContactLeadUseCase struct {
	repo ports.LeadRepository
}

func NewContactLeadUseCase(repo ports.LeadRepository) *ContactLeadUseCase {
	return &ContactLeadUseCase{repo: repo}
}

func (uc *ContactLeadUseCase) Execute(ctx context.Context, leadID string) (*LeadDTO, error) {
	lead, err := uc.repo.GetByID(ctx, leadID)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, apperr.NewNotFound("lead no encontrado", nil)
	}

	if err := lead.MarkContacted(); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil, apperr.NewPreconditionFailed(err.Error(), err)
		}
		return nil, err
	}

	// "Contactado" no es uno de los eventos de integración definidos para CRM
	// (solo crm.lead.created y crm.lead.visit_scheduled) — se persiste el
	// estado sin encolar ningún evento nuevo en el outbox.
	if err := uc.repo.Save(ctx, lead, "", nil); err != nil {
		return nil, err
	}

	return toLeadDTO(lead), nil
}
