package application

import (
	"context"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

type ApproveTicketUseCase struct {
	repo       ports.TicketRepository
	dispatcher ports.EventDispatcher
}

func NewApproveTicketUseCase(repo ports.TicketRepository, dispatcher ports.EventDispatcher) *ApproveTicketUseCase {
	return &ApproveTicketUseCase{repo: repo, dispatcher: dispatcher}
}

func (uc *ApproveTicketUseCase) Execute(ctx context.Context, ticketID string) error {
	ticket, err := uc.repo.FindByID(ctx, ticketID)
	if err != nil || ticket == nil {
		return ErrTicketNotFound
	}

	if err := ticket.Approve(); err != nil {
		return err
	}

	// Transición interna automática para dejarlo listo para arrancar la obra
	_ = ticket.StartWork() // Pasa de APPROVED a IN_PROGRESS de forma directa para ejecución

	if err := uc.repo.Save(ctx, ticket); err != nil {
		return err
	}

	// Despachar evento colateral hacia el exterior
	event := domain.TicketApprovedEvent{
		TicketID:   ticket.ID,
		PropertyID: ticket.PropertyID,
		ApprovedAt: ticket.Quote.QuotedAt,
	}
	return uc.dispatcher.DispatchApproved(ctx, event)
}
