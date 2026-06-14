package domain

import (
	"crypto/rand"
	"fmt"
	"time"

	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

// ── Tipos ────────────────────────────────────────────────────────────────

type MessageType string

const (
	MessageTypeText          MessageType = "TEXT"
	MessageTypeVisitProposal MessageType = "VISIT_PROPOSAL"
	MessageTypeSystem        MessageType = "SYSTEM"
)

type VisitProposalStatus string

const (
	VisitProposalPending  VisitProposalStatus = "PENDING_APPROVAL"
	VisitProposalAccepted VisitProposalStatus = "ACCEPTED"
	VisitProposalRejected VisitProposalStatus = "REJECTED"
)

// ── Conversation ─────────────────────────────────────────────────────────

// Conversation es el agregado raíz del contexto de chat.
// Representa el hilo de comunicación entre un Buscador y un Anunciante
// sobre una propiedad específica.
type Conversation struct {
	ddd.AggregateRoot
	id             string
	propertyID     string
	propertyTitle  string // denormalizado para mostrar en la lista sin llamadas cross-service
	seekerID       string // el Buscador que inició la consulta
	seekerName     string // denormalizado
	advertiserID   string // el Propietario o Agente
	advertiserName string // denormalizado
	leadID         string // referencia al Lead en CRM (puede estar vacío inicialmente)
	createdAt      time.Time
	updatedAt      time.Time
}

func NewConversation(propertyID, propertyTitle, seekerID, seekerName, advertiserID, advertiserName string) (*Conversation, error) {
	if propertyID == "" || seekerID == "" || advertiserID == "" {
		return nil, apperr.NewBadRequest("property_id, seeker_id y advertiser_id son obligatorios", nil)
	}
	if seekerID == advertiserID {
		return nil, apperr.NewBadRequest("el buscador y el anunciante no pueden ser el mismo usuario", nil)
	}

	now := time.Now().UTC()
	c := &Conversation{
		id:             nextID(),
		propertyID:     propertyID,
		propertyTitle:  propertyTitle,
		seekerID:       seekerID,
		seekerName:     seekerName,
		advertiserID:   advertiserID,
		advertiserName: advertiserName,
		createdAt:      now,
		updatedAt:      now,
	}
	c.RecordEvent(NewConversationStarted(c))
	return c, nil
}

// ReconstructConversation reconstruye desde persistencia sin disparar eventos.
func ReconstructConversation(
	id, propertyID, propertyTitle, seekerID, seekerName, advertiserID, advertiserName, leadID string,
	createdAt, updatedAt time.Time,
) *Conversation {
	return &Conversation{
		id:             id,
		propertyID:     propertyID,
		propertyTitle:  propertyTitle,
		seekerID:       seekerID,
		seekerName:     seekerName,
		advertiserID:   advertiserID,
		advertiserName: advertiserName,
		leadID:         leadID,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
	}
}

// LinkLead asocia el Lead de CRM a esta conversación.
func (c *Conversation) LinkLead(leadID string) {
	c.leadID = leadID
	c.updatedAt = time.Now().UTC()
}

// IsParticipant verifica que el userID sea parte de la conversación.
func (c *Conversation) IsParticipant(userID string) bool {
	return c.seekerID == userID || c.advertiserID == userID
}

func (c *Conversation) Touch() { c.updatedAt = time.Now().UTC() }

func (c *Conversation) ID() string              { return c.id }
func (c *Conversation) PropertyID() string      { return c.propertyID }
func (c *Conversation) PropertyTitle() string   { return c.propertyTitle }
func (c *Conversation) SeekerID() string        { return c.seekerID }
func (c *Conversation) SeekerName() string      { return c.seekerName }
func (c *Conversation) AdvertiserID() string    { return c.advertiserID }
func (c *Conversation) AdvertiserName() string  { return c.advertiserName }
func (c *Conversation) LeadID() string          { return c.leadID }
func (c *Conversation) CreatedAt() time.Time    { return c.createdAt }
func (c *Conversation) UpdatedAt() time.Time    { return c.updatedAt }

// ── Message ───────────────────────────────────────────────────────────────

// Message es una entidad interna de Conversation.
type Message struct {
	id             string
	conversationID string
	senderID       string
	msgType        MessageType
	body           string
	metadata       map[string]string // para tarjetas interactivas
	createdAt      time.Time
}

func NewTextMessage(conversationID, senderID, body string) (*Message, error) {
	if conversationID == "" || senderID == "" {
		return nil, apperr.NewBadRequest("conversation_id y sender_id son obligatorios", nil)
	}
	if body == "" {
		return nil, apperr.NewBadRequest("el cuerpo del mensaje no puede estar vacío", nil)
	}
	if len(body) > 4000 {
		return nil, apperr.NewBadRequest("el mensaje no puede superar los 4000 caracteres", nil)
	}
	return &Message{
		id:             nextID(),
		conversationID: conversationID,
		senderID:       senderID,
		msgType:        MessageTypeText,
		body:           body,
		metadata:       map[string]string{},
		createdAt:      time.Now().UTC(),
	}, nil
}

// NewVisitProposalMessage crea un mensaje especial de tipo VISIT_PROPOSAL.
func NewVisitProposalMessage(conversationID, senderID, visitProposalID, proposedAt string) (*Message, error) {
	if conversationID == "" || senderID == "" || visitProposalID == "" || proposedAt == "" {
		return nil, apperr.NewBadRequest("todos los campos de la propuesta de visita son obligatorios", nil)
	}
	return &Message{
		id:             nextID(),
		conversationID: conversationID,
		senderID:       senderID,
		msgType:        MessageTypeVisitProposal,
		body:           "Propuesta de visita",
		metadata: map[string]string{
			"visit_proposal_id": visitProposalID,
			"proposed_at":       proposedAt,
			"status":            string(VisitProposalPending),
		},
		createdAt: time.Now().UTC(),
	}, nil
}

// NewSystemMessage crea un mensaje automático del sistema (ej: visita confirmada).
func NewSystemMessage(conversationID, body string, meta map[string]string) *Message {
	if meta == nil {
		meta = map[string]string{}
	}
	return &Message{
		id:             nextID(),
		conversationID: conversationID,
		senderID:       "system",
		msgType:        MessageTypeSystem,
		body:           body,
		metadata:       meta,
		createdAt:      time.Now().UTC(),
	}
}

// ReconstructMessage reconstruye desde persistencia.
func ReconstructMessage(id, conversationID, senderID string, msgType MessageType, body string, metadata map[string]string, createdAt time.Time) *Message {
	if metadata == nil {
		metadata = map[string]string{}
	}
	return &Message{
		id:             id,
		conversationID: conversationID,
		senderID:       senderID,
		msgType:        msgType,
		body:           body,
		metadata:       metadata,
		createdAt:      createdAt,
	}
}

func (m *Message) ID() string                     { return m.id }
func (m *Message) ConversationID() string         { return m.conversationID }
func (m *Message) SenderID() string               { return m.senderID }
func (m *Message) Type() MessageType              { return m.msgType }
func (m *Message) Body() string                   { return m.body }
func (m *Message) Metadata() map[string]string    { return m.metadata }
func (m *Message) CreatedAt() time.Time           { return m.createdAt }

// ── VisitProposal ─────────────────────────────────────────────────────────

// VisitProposal representa la tarjeta interactiva de propuesta de visita
// embebida en el chat. Es una entidad independiente referenciada desde Message.
type VisitProposal struct {
	id             string
	conversationID string
	leadID         string
	proposedAt     time.Time // fecha y hora propuesta para la visita
	status         VisitProposalStatus
	resolvedAt     *time.Time
	createdAt      time.Time
}

func NewVisitProposal(conversationID, leadID string, proposedAt time.Time) (*VisitProposal, error) {
	if conversationID == "" || leadID == "" {
		return nil, apperr.NewBadRequest("conversation_id y lead_id son obligatorios", nil)
	}
	if proposedAt.Before(time.Now().UTC()) {
		return nil, apperr.NewBadRequest("la fecha de visita propuesta debe ser futura", nil)
	}
	return &VisitProposal{
		id:             nextID(),
		conversationID: conversationID,
		leadID:         leadID,
		proposedAt:     proposedAt,
		status:         VisitProposalPending,
		createdAt:      time.Now().UTC(),
	}, nil
}

func ReconstructVisitProposal(id, conversationID, leadID string, proposedAt time.Time, status VisitProposalStatus, resolvedAt *time.Time, createdAt time.Time) *VisitProposal {
	return &VisitProposal{
		id:             id,
		conversationID: conversationID,
		leadID:         leadID,
		proposedAt:     proposedAt,
		status:         status,
		resolvedAt:     resolvedAt,
		createdAt:      createdAt,
	}
}

func (v *VisitProposal) Accept() error {
	if v.status != VisitProposalPending {
		return apperr.NewPreconditionFailed("solo se puede aceptar una propuesta PENDING_APPROVAL", nil)
	}
	now := time.Now().UTC()
	v.status = VisitProposalAccepted
	v.resolvedAt = &now
	return nil
}

func (v *VisitProposal) Reject() error {
	if v.status != VisitProposalPending {
		return apperr.NewPreconditionFailed("solo se puede rechazar una propuesta PENDING_APPROVAL", nil)
	}
	now := time.Now().UTC()
	v.status = VisitProposalRejected
	v.resolvedAt = &now
	return nil
}

func (v *VisitProposal) ID() string                   { return v.id }
func (v *VisitProposal) ConversationID() string       { return v.conversationID }
func (v *VisitProposal) LeadID() string               { return v.leadID }
func (v *VisitProposal) ProposedAt() time.Time        { return v.proposedAt }
func (v *VisitProposal) Status() VisitProposalStatus  { return v.status }
func (v *VisitProposal) ResolvedAt() *time.Time       { return v.resolvedAt }
func (v *VisitProposal) CreatedAt() time.Time         { return v.createdAt }

// ── Helper ────────────────────────────────────────────────────────────────

func nextID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
