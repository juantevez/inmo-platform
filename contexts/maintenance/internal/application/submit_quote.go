package application

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/ports"
)

type SubmitQuoteCommand struct {
	TicketID string  `json:"ticket_id"`
	Amount   float64 `json:"amount"`
	Details  string  `json:"details"`
}

type SubmitQuoteUseCase struct {
	repo ports.TicketRepository
}

func NewSubmitQuoteUseCase(repo ports.TicketRepository) *SubmitQuoteUseCase {
	return &SubmitQuoteUseCase{repo: repo}
}

func (uc *SubmitQuoteUseCase) Execute(ctx context.Context, cmd SubmitQuoteCommand) error {
	ticket, err := uc.repo.FindByID(ctx, cmd.TicketID)
	if err != nil || ticket == nil {
		return ErrTicketNotFound
	}

	// El dominio defiende la invariante de que exista un proveedor asignado
	if err := ticket.SubmitQuote(cmd.Amount, cmd.Details); err != nil {
		return err
	}

	return uc.repo.Save(ctx, ticket)
}
