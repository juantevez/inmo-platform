package application

import (
	"context"
	"errors"

	"inmo.platform/contexts/maintenance/internal/ports"
)

var ErrTicketNotFound = errors.New("el ticket de mantenimiento no existe")

type AssignProviderCommand struct {
	TicketID   string `json:"ticket_id"`
	ProviderID string `json:"provider_id"`
}

type AssignProviderUseCase struct {
	repo ports.TicketRepository
}

func NewAssignProviderUseCase(repo ports.TicketRepository) *AssignProviderUseCase {
	return &AssignProviderUseCase{repo: repo}
}

func (uc *AssignProviderUseCase) Execute(ctx context.Context, cmd AssignProviderCommand) error {
	ticket, err := uc.repo.FindByID(ctx, cmd.TicketID)
	if err != nil || ticket == nil {
		return ErrTicketNotFound
	}

	// Mutación controlada por el dominio
	if err := ticket.AssignProvider(cmd.ProviderID); err != nil {
		return err
	}

	return uc.repo.Save(ctx, ticket)
}
