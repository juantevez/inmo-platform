package application

import (
	"context"
	"errors"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

var ErrPropertyNotFound = errors.New("la propiedad especificada no existe o no está activa")

type CreateTicketCommand struct {
	ID          string `json:"id"`
	PropertyID  string `json:"property_id"`
	TenantID    string `json:"tenant_id"`
	Description string `json:"description"`
	Urgency     string `json:"urgency"` // "EMERGENCY", "URGENT", "SCHEDULED"
}

type CreateTicketUseCase struct {
	repo           ports.TicketRepository
	catalogService ports.CatalogService
}

func NewCreateTicketUseCase(repo ports.TicketRepository, catalog ports.CatalogService) *CreateTicketUseCase {
	return &CreateTicketUseCase{repo: repo, catalogService: catalog}
}

func (uc *CreateTicketUseCase) Execute(ctx context.Context, cmd CreateTicketCommand) error {
	// 1. Validar consistencia con el contexto de Catálogo
	exists, err := uc.catalogService.PropertyExists(ctx, cmd.PropertyID)
	if err != nil || !exists {
		return ErrPropertyNotFound
	}

	// 2. Construir el Agregado en su estado inicial (OPEN)
	ticket := domain.NewTicket(
		cmd.ID,
		cmd.PropertyID,
		cmd.TenantID,
		cmd.Description,
		domain.UrgencyLevel(cmd.Urgency),
	)

	// 3. Persistir en la base de datos
	return uc.repo.Save(ctx, ticket)
}
