package application

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

type CloseTicketCommand struct {
	TicketID    string `json:"ticket_id"`
	Description string `json:"description"`
	DocumentURL string `json:"document_url"`
}

type CloseTicketUseCase struct {
	repo       ports.TicketRepository
	dispatcher ports.EventDispatcher
}

func NewCloseTicketUseCase(repo ports.TicketRepository, dispatcher ports.EventDispatcher) *CloseTicketUseCase {
	return &CloseTicketUseCase{repo: repo, dispatcher: dispatcher}
}

func (uc *CloseTicketUseCase) Execute(ctx context.Context, cmd CloseTicketCommand) error {
	ticket, err := uc.repo.FindByID(ctx, cmd.TicketID)
	if err != nil || ticket == nil {
		return ErrTicketNotFound
	}

	// El dominio exige descripción y foto de AWS S3 obligatoria
	if err := ticket.Close(cmd.Description, cmd.DocumentURL); err != nil {
		return err
	}

	if err := uc.repo.Save(ctx, ticket); err != nil {
		return err
	}

	// Despachar evento de liberación e historial
	event := domain.TicketClosedEvent{
		TicketID:    ticket.ID,
		PropertyID:  ticket.PropertyID,
		ProviderID:  ticket.ProviderID,
		Cost:        ticket.Quote.Amount,
		Description: ticket.Evidence.Description,
		ClosedAt:    ticket.Evidence.ClosedAt,
	}
	return uc.dispatcher.DispatchClosed(ctx, event)
}
