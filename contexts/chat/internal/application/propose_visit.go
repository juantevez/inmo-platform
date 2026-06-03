package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type ProposeVisitUseCase struct {
	convRepo     ports.ConversationRepository
	msgRepo      ports.MessageRepository
	proposalRepo ports.VisitProposalRepository
	hub          ports.WebSocketHub
	publisher    ports.EventPublisher
}

func NewProposeVisitUseCase(
	convRepo ports.ConversationRepository,
	msgRepo ports.MessageRepository,
	proposalRepo ports.VisitProposalRepository,
	hub ports.WebSocketHub,
	publisher ports.EventPublisher,
) *ProposeVisitUseCase {
	return &ProposeVisitUseCase{
		convRepo:     convRepo,
		msgRepo:      msgRepo,
		proposalRepo: proposalRepo,
		hub:          hub,
		publisher:    publisher,
	}
}

type ProposeVisitCommand struct {
	ConversationID string
	SeekerID       string
	LeadID         string
	ProposedAt     time.Time
}

type VisitProposalDTO struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	LeadID         string `json:"lead_id"`
	ProposedAt     string `json:"proposed_at"`
	Status         string `json:"status"`
	MessageID      string `json:"message_id"`
}

func (uc *ProposeVisitUseCase) Execute(ctx context.Context, cmd ProposeVisitCommand) (*VisitProposalDTO, error) {
	// 1. Validar conversación y participante
	conv, err := uc.convRepo.FindByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, apperr.NewNotFound("conversación no encontrada", nil)
	}
	if conv.SeekerID() != cmd.SeekerID {
		return nil, apperr.NewForbidden("solo el buscador puede proponer una visita", nil)
	}

	// 2. Crear la propuesta de visita en el dominio
	proposal, err := domain.NewVisitProposal(cmd.ConversationID, cmd.LeadID, cmd.ProposedAt)
	if err != nil {
		return nil, err
	}

	// 3. Crear el mensaje especial asociado
	msg, err := domain.NewVisitProposalMessage(
		cmd.ConversationID,
		cmd.SeekerID,
		proposal.ID(),
		proposal.ProposedAt().Format("2006-01-02T15:04:05Z"),
	)
	if err != nil {
		return nil, err
	}

	// 4. Persistir propuesta y mensaje
	if err := uc.proposalRepo.Save(ctx, proposal); err != nil {
		return nil, fmt.Errorf("error al guardar la propuesta de visita: %w", err)
	}
	if err := uc.msgRepo.Save(ctx, msg); err != nil {
		return nil, fmt.Errorf("error al guardar el mensaje de propuesta: %w", err)
	}

	conv.Touch()
	_ = uc.convRepo.Save(ctx, conv)

	dto := &VisitProposalDTO{
		ID:             proposal.ID(),
		ConversationID: proposal.ConversationID(),
		LeadID:         proposal.LeadID(),
		ProposedAt:     proposal.ProposedAt().Format("2006-01-02T15:04:05Z"),
		Status:         string(proposal.Status()),
		MessageID:      msg.ID(),
	}

	// 5. Publicar evento y broadcast (best-effort)
	go func() {
		event := domain.NewVisitProposed(proposal)
		payload, err := json.Marshal(event)
		if err == nil {
			_ = uc.publisher.Publish(context.Background(), "chat.visit.proposed", payload)
		}
		wsPayload, err := json.Marshal(toMessageDTO(msg))
		if err == nil {
			uc.hub.Broadcast(cmd.ConversationID, wsPayload)
		}
	}()

	return dto, nil
}
