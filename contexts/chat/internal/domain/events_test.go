package domain_test

import (
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
)

// ─── NewConversationStarted ─────────────────────────────────────────────────

func TestNewConversationStarted_MapeaCamposYMetadataDelEvento(t *testing.T) {
	// Se usa ReconstructConversation para poder ejercitar el constructor del
	// evento de forma aislada, sin el evento que NewConversation ya registra
	// automáticamente al crear el agregado.
	conv := domain.ReconstructConversation("conv-1", "prop-1", "Depto", "seeker-1", "Juan", "advertiser-1", "Ana", "", time.Now(), time.Now())

	evt := domain.NewConversationStarted(conv)

	if evt.EventName() != "chat.conversation.started" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != "conv-1" {
		t.Errorf("AggregateID: got %q, want %q", evt.AggregateID(), "conv-1")
	}
	if evt.PropertyID != "prop-1" || evt.SeekerID != "seeker-1" || evt.AdvertiserID != "advertiser-1" {
		t.Errorf("evento: got %+v", evt)
	}
	if !idPattern.MatchString(evt.EventID()) {
		t.Errorf("EventID: got %q, no matchea el formato esperado", evt.EventID())
	}
	if evt.OccurredAt().IsZero() || time.Since(evt.OccurredAt()) > time.Minute {
		t.Errorf("OccurredAt: got %v, want un timestamp reciente", evt.OccurredAt())
	}
}

// ─── NewMessageSent ─────────────────────────────────────────────────────────

func TestNewMessageSent_MapeaCamposYUsaConversationIDComoAggregate(t *testing.T) {
	msg, err := domain.NewTextMessage("conv-1", "sender-1", "Hola, ¿sigue disponible?")
	if err != nil {
		t.Fatalf("NewTextMessage: %v", err)
	}

	evt := domain.NewMessageSent(msg)

	if evt.EventName() != "chat.message.sent" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	// A diferencia de otros eventos, el aggregate de MessageSent es la CONVERSACIÓN
	// (no el mensaje) — tiene sentido de negocio: lo que se sigue/suscribe es el hilo.
	if evt.AggregateID() != "conv-1" {
		t.Errorf("AggregateID: got %q, want %q (conversation_id, no message_id)", evt.AggregateID(), "conv-1")
	}
	if evt.ConversationID != "conv-1" || evt.SenderID != "sender-1" || evt.Body != "Hola, ¿sigue disponible?" {
		t.Errorf("evento: got %+v", evt)
	}
	if evt.MessageType != string(domain.MessageTypeText) {
		t.Errorf("MessageType: got %q, want %q", evt.MessageType, domain.MessageTypeText)
	}
}

func TestNewMessageSent_PropagaMetadataDeVisitProposal(t *testing.T) {
	msg, err := domain.NewVisitProposalMessage("conv-1", "sender-1", "vp-1", "2025-01-01T10:00:00Z")
	if err != nil {
		t.Fatalf("NewVisitProposalMessage: %v", err)
	}

	evt := domain.NewMessageSent(msg)

	if evt.Metadata["visit_proposal_id"] != "vp-1" {
		t.Errorf("Metadata: got %v, want que incluya visit_proposal_id", evt.Metadata)
	}
	if evt.MessageType != string(domain.MessageTypeVisitProposal) {
		t.Errorf("MessageType: got %q, want %q", evt.MessageType, domain.MessageTypeVisitProposal)
	}
}

// ─── NewVisitProposed ───────────────────────────────────────────────────────

func TestNewVisitProposed_MapeaCamposYFormateaLaFecha(t *testing.T) {
	// proposedAt debe ser futura (NewVisitProposal lo exige) — se calcula en relativo
	// al momento del test en vez de hardcodear una fecha que eventualmente quedaría en el pasado.
	proposedAt := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", proposedAt)
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}

	evt := domain.NewVisitProposed(vp)

	if evt.EventName() != "chat.visit.proposed" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != vp.ID() {
		t.Errorf("AggregateID: got %q, want %q (el ID de la propuesta)", evt.AggregateID(), vp.ID())
	}
	if evt.ConversationID != "conv-1" || evt.LeadID != "lead-1" {
		t.Errorf("evento: got %+v", evt)
	}
	wantFormatted := proposedAt.Format("2006-01-02T15:04:05Z")
	if evt.ProposedAt != wantFormatted {
		t.Errorf("ProposedAt: got %q, want %q", evt.ProposedAt, wantFormatted)
	}
}

// ─── nextID (indirecto) ─────────────────────────────────────────────────────

func TestEventos_GeneranEventIDsUnicos(t *testing.T) {
	conv := domain.ReconstructConversation("conv-1", "prop-1", "Depto", "seeker-1", "Juan", "advertiser-1", "Ana", "", time.Now(), time.Now())

	evt1 := domain.NewConversationStarted(conv)
	evt2 := domain.NewConversationStarted(conv)

	if evt1.EventID() == evt2.EventID() {
		t.Errorf("EventID: dos eventos distintos generaron el mismo ID (%q)", evt1.EventID())
	}
}
