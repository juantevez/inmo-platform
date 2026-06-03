package ports

import (
	"context"

	"inmo.platform/contexts/chat/internal/domain"
)

// ConversationRepository define el contrato de persistencia del agregado Conversation.
type ConversationRepository interface {
	Save(ctx context.Context, c *domain.Conversation) error
	FindByID(ctx context.Context, id string) (*domain.Conversation, error)
	// FindByParticipant devuelve todas las conversaciones donde el userID es seeker o advertiser.
	FindByParticipant(ctx context.Context, userID string) ([]*domain.Conversation, error)
	// FindByPropertyAndParticipants busca si ya existe un hilo para esa propiedad entre esos dos usuarios.
	FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error)
}

// MessageRepository define el contrato de persistencia de los mensajes.
type MessageRepository interface {
	Save(ctx context.Context, m *domain.Message) error
	FindByConversation(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error)
}

// VisitProposalRepository define el contrato de persistencia de las propuestas de visita.
type VisitProposalRepository interface {
	Save(ctx context.Context, v *domain.VisitProposal) error
	FindByID(ctx context.Context, id string) (*domain.VisitProposal, error)
	Update(ctx context.Context, v *domain.VisitProposal) error
}

// OutboxRepository define el contrato para persistir eventos de forma atómica (Outbox Pattern).
type OutboxRepository interface {
	SaveTx(ctx context.Context, tx interface{}, subject string, payload []byte) error
}

// EventPublisher define el contrato para publicar eventos a NATS JetStream.
type EventPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// WebSocketHub define el contrato para empujar mensajes a clientes conectados.
type WebSocketHub interface {
	// Broadcast envía el payload JSON a todos los clientes suscritos a esa conversación.
	Broadcast(conversationID string, payload []byte)
}
