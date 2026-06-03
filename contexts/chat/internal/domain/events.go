package domain

import "inmo.platform/shared/pkg/ddd"

// ── Eventos de Conversation ───────────────────────────────────────────────

type ConversationStartedEvent struct {
	ddd.BaseDomainEvent
	PropertyID   string `json:"property_id"`
	SeekerID     string `json:"seeker_id"`
	AdvertiserID string `json:"advertiser_id"`
}

func NewConversationStarted(c *Conversation) ConversationStartedEvent {
	return ConversationStartedEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(nextID(), c.ID(), "chat.conversation.started"),
		PropertyID:      c.PropertyID(),
		SeekerID:        c.SeekerID(),
		AdvertiserID:    c.AdvertiserID(),
	}
}

// ── Eventos de Message ────────────────────────────────────────────────────

type MessageSentEvent struct {
	ddd.BaseDomainEvent
	ConversationID string            `json:"conversation_id"`
	SenderID       string            `json:"sender_id"`
	MessageType    string            `json:"message_type"`
	Body           string            `json:"body"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func NewMessageSent(m *Message) MessageSentEvent {
	return MessageSentEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(nextID(), m.ConversationID(), "chat.message.sent"),
		ConversationID:  m.ConversationID(),
		SenderID:        m.SenderID(),
		MessageType:     string(m.Type()),
		Body:            m.Body(),
		Metadata:        m.Metadata(),
	}
}

// ── Eventos de VisitProposal ──────────────────────────────────────────────

type VisitProposedEvent struct {
	ddd.BaseDomainEvent
	ConversationID string `json:"conversation_id"`
	LeadID         string `json:"lead_id"`
	ProposedAt     string `json:"proposed_at"`
}

func NewVisitProposed(v *VisitProposal) VisitProposedEvent {
	return VisitProposedEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(nextID(), v.ID(), "chat.visit.proposed"),
		ConversationID:  v.ConversationID(),
		LeadID:          v.LeadID(),
		ProposedAt:      v.ProposedAt().Format("2006-01-02T15:04:05Z"),
	}
}
