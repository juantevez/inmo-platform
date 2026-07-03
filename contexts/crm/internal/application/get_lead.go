package application

import (
	"context"
	"time"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// LeadDTO es la representación de salida del agregado Lead para la capa HTTP.
type LeadDTO struct {
	ID               string  `json:"id"`
	PropertyID       string  `json:"property_id"`
	ClientName       string  `json:"client_name"`
	Email            string  `json:"email,omitempty"`
	Phone            string  `json:"phone,omitempty"`
	State            string  `json:"state"`
	VisitScheduledAt *string `json:"visit_scheduled_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func toLeadDTO(l *domain.Lead) *LeadDTO {
	dto := &LeadDTO{
		ID:         l.ID,
		PropertyID: l.PropertyID,
		ClientName: l.ClientName,
		Email:      l.Email,
		Phone:      l.Phone,
		State:      string(l.State),
		CreatedAt:  l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  l.UpdatedAt.Format(time.RFC3339),
	}
	if l.VisitScheduledAt != nil {
		formatted := l.VisitScheduledAt.Format(time.RFC3339)
		dto.VisitScheduledAt = &formatted
	}
	return dto
}

type GetLeadUseCase struct {
	repo ports.LeadRepository
}

func NewGetLeadUseCase(repo ports.LeadRepository) *GetLeadUseCase {
	return &GetLeadUseCase{repo: repo}
}

func (uc *GetLeadUseCase) Execute(ctx context.Context, leadID string) (*LeadDTO, error) {
	lead, err := uc.repo.GetByID(ctx, leadID)
	if err != nil {
		return nil, err
	}
	if lead == nil {
		return nil, apperr.NewNotFound("lead no encontrado", nil)
	}
	return toLeadDTO(lead), nil
}
