package application

import (
	"context"
	"fmt"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// ── ListConversationsUseCase ──────────────────────────────────────────────

type ListConversationsUseCase struct {
	convRepo ports.ConversationRepository
}

func NewListConversationsUseCase(convRepo ports.ConversationRepository) *ListConversationsUseCase {
	return &ListConversationsUseCase{convRepo: convRepo}
}

func (uc *ListConversationsUseCase) Execute(ctx context.Context, userID string) ([]*ConversationDTO, error) {
	if userID == "" {
		return nil, apperr.NewBadRequest("user_id es requerido", nil)
	}

	summaries, err := uc.convRepo.FindByParticipant(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("error al listar conversaciones: %w", err)
	}

	dtos := make([]*ConversationDTO, 0, len(summaries))
	for _, s := range summaries {
		dtos = append(dtos, toConversationDTO(s.Conversation, userID, s.LastMessage))
	}
	return dtos, nil
}

// ── GetMessagesUseCase ────────────────────────────────────────────────────

type GetMessagesUseCase struct {
	convRepo ports.ConversationRepository
	msgRepo  ports.MessageRepository
}

func NewGetMessagesUseCase(convRepo ports.ConversationRepository, msgRepo ports.MessageRepository) *GetMessagesUseCase {
	return &GetMessagesUseCase{convRepo: convRepo, msgRepo: msgRepo}
}

type MessageDTO struct {
	ID             string            `json:"id"`
	ConversationID string            `json:"conversation_id"`
	SenderID       string            `json:"sender_id"`
	Type           string            `json:"type"`
	Body           string            `json:"body"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      string            `json:"created_at"`
}

func (uc *GetMessagesUseCase) Execute(ctx context.Context, conversationID, requesterID string, limit, offset int) ([]*MessageDTO, error) {
	conv, err := uc.convRepo.FindByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, apperr.NewNotFound("conversación no encontrada", nil)
	}
	if !conv.IsParticipant(requesterID) {
		return nil, apperr.NewForbidden("solo los participantes pueden leer los mensajes", nil)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	msgs, err := uc.msgRepo.FindByConversation(ctx, conversationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error al obtener mensajes: %w", err)
	}

	dtos := make([]*MessageDTO, 0, len(msgs))
	for _, m := range msgs {
		dtos = append(dtos, toMessageDTO(m))
	}
	return dtos, nil
}

func toMessageDTO(m *domain.Message) *MessageDTO {
	return &MessageDTO{
		ID:             m.ID(),
		ConversationID: m.ConversationID(),
		SenderID:       m.SenderID(),
		Type:           string(m.Type()),
		Body:           m.Body(),
		Metadata:       m.Metadata(),
		CreatedAt:      m.CreatedAt().Format("2006-01-02T15:04:05Z"),
	}
}
