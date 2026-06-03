package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type SendMessageUseCase struct {
	db       *sql.DB
	convRepo ports.ConversationRepository
	msgRepo  ports.MessageRepository
	hub      ports.WebSocketHub
	publisher ports.EventPublisher
}

func NewSendMessageUseCase(
	db *sql.DB,
	convRepo ports.ConversationRepository,
	msgRepo ports.MessageRepository,
	hub ports.WebSocketHub,
	publisher ports.EventPublisher,
) *SendMessageUseCase {
	return &SendMessageUseCase{
		db:        db,
		convRepo:  convRepo,
		msgRepo:   msgRepo,
		hub:       hub,
		publisher: publisher,
	}
}

type SendMessageCommand struct {
	ConversationID string
	SenderID       string
	Body           string
}

func (uc *SendMessageUseCase) Execute(ctx context.Context, cmd SendMessageCommand) (*MessageDTO, error) {
	// 1. Validar que la conversación exista y el sender sea participante
	conv, err := uc.convRepo.FindByID(ctx, cmd.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, apperr.NewNotFound("conversación no encontrada", nil)
	}
	if !conv.IsParticipant(cmd.SenderID) {
		return nil, apperr.NewForbidden("solo los participantes pueden enviar mensajes", nil)
	}

	// 2. Crear el mensaje en el dominio
	msg, err := domain.NewTextMessage(cmd.ConversationID, cmd.SenderID, cmd.Body)
	if err != nil {
		return nil, err
	}

	// 3. Persistir el mensaje
	if err := uc.msgRepo.Save(ctx, msg); err != nil {
		return nil, fmt.Errorf("error al guardar el mensaje: %w", err)
	}

	// 4. Actualizar updated_at de la conversación
	conv.Touch()
	if err := uc.convRepo.Save(ctx, conv); err != nil {
		return nil, fmt.Errorf("error al actualizar la conversación: %w", err)
	}

	dto := toMessageDTO(msg)

	// 5. Publicar a NATS y hacer broadcast WebSocket (best-effort, no bloquea)
	go func() {
		event := domain.NewMessageSent(msg)
		payload, err := json.Marshal(event)
		if err == nil {
			_ = uc.publisher.Publish(context.Background(), "chat.message.sent", payload)
		}
		wsPayload, err := json.Marshal(dto)
		if err == nil {
			uc.hub.Broadcast(cmd.ConversationID, wsPayload)
		}
	}()

	return dto, nil
}
