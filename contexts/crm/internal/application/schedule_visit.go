package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// ScheduleVisitDTO son los datos de entrada para agendar una visita.
type ScheduleVisitDTO struct {
	LeadID  string
	VisitAt time.Time
}

// ScheduleVisitUseCase agenda una visita para el lead y publica crm.lead.visit_scheduled.
type ScheduleVisitUseCase struct {
	repo ports.LeadRepository
}

func NewScheduleVisitUseCase(repo ports.LeadRepository) *ScheduleVisitUseCase {
	return &ScheduleVisitUseCase{repo: repo}
}

func (uc *ScheduleVisitUseCase) Execute(ctx context.Context, dto ScheduleVisitDTO) (*LeadDTO, error) {
	lead, err := uc.repo.GetByID(ctx, dto.LeadID)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, apperr.NewNotFound("lead no encontrado", nil)
	}

	if err := lead.ScheduleVisit(dto.VisitAt); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil, apperr.NewPreconditionFailed(err.Error(), err)
		}
		// Cualquier otro error (invariante de fecha futura) ya viene como
		// *apperr.AppError de BadRequest directamente desde el dominio.
		return nil, err
	}

	eventPayload, err := json.Marshal(map[string]interface{}{
		"id":                 lead.ID,
		"property_id":        lead.PropertyID,
		"client_name":        lead.ClientName,
		"email":              lead.Email,
		"phone":              lead.Phone,
		"visit_scheduled_at": lead.VisitScheduledAt,
		"state":              string(lead.State),
	})
	if err != nil {
		return nil, apperr.NewInternal(fmt.Sprintf("error al serializar evento de visita agendada: %v", err), err)
	}

	if err := uc.repo.Save(ctx, lead, "crm.lead.visit_scheduled", eventPayload); err != nil {
		return nil, err
	}

	return toLeadDTO(lead), nil
}
