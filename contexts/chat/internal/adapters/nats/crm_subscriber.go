package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go/jetstream"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
)

// visitScheduledPayload refleja el evento crm.lead.visit_scheduled publicado por el CRM.
type visitScheduledPayload struct {
	LeadID         string `json:"lead_id"`
	ConversationID string `json:"conversation_id"`
	ProposalID     string `json:"proposal_id"`
	ScheduledAt    string `json:"scheduled_at"`
}

// visitRejectedPayload refleja el evento crm.lead.visit_rejected publicado por el CRM.
type visitRejectedPayload struct {
	LeadID         string `json:"lead_id"`
	ConversationID string `json:"conversation_id"`
	ProposalID     string `json:"proposal_id"`
	Reason         string `json:"reason"`
}

// CRMEventSubscriber escucha eventos del CRM e inyecta mensajes de sistema en el chat.
type CRMEventSubscriber struct {
	js           jetstream.JetStream
	proposalRepo ports.VisitProposalRepository
	msgRepo      ports.MessageRepository
	convRepo     ports.ConversationRepository
	hub          ports.WebSocketHub
}

func NewCRMEventSubscriber(
	js jetstream.JetStream,
	proposalRepo ports.VisitProposalRepository,
	msgRepo ports.MessageRepository,
	convRepo ports.ConversationRepository,
	hub ports.WebSocketHub,
) *CRMEventSubscriber {
	return &CRMEventSubscriber{
		js:           js,
		proposalRepo: proposalRepo,
		msgRepo:      msgRepo,
		convRepo:     convRepo,
		hub:          hub,
	}
}

func (s *CRMEventSubscriber) StartConsume(ctx context.Context) error {
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "crm", jetstream.ConsumerConfig{
		Durable:       "chat-crm-visit-sync",
		FilterSubject: "crm.lead.visit_*",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("error al crear consumidor CRM en chat: %w", err)
	}

	log.Println("[CHAT NATS] 📡 Escuchando eventos de visitas en 'crm.lead.visit_*'...")

	iter, err := cons.Messages()
	if err != nil {
		return err
	}

	go func() {
		for {
			msg, err := iter.Next()
			if err != nil {
				log.Printf("[CHAT NATS ERROR] %v\n", err)
				return
			}

			var processErr error
			switch msg.Subject() {
			case "crm.lead.visit_scheduled":
				processErr = s.handleVisitScheduled(ctx, msg.Data())
			case "crm.lead.visit_rejected":
				processErr = s.handleVisitRejected(ctx, msg.Data())
			}

			if processErr != nil {
				log.Printf("[CHAT NATS ERROR] Error procesando %s: %v\n", msg.Subject(), processErr)
				continue
			}
			_ = msg.Ack()
		}
	}()

	return nil
}

func (s *CRMEventSubscriber) handleVisitScheduled(ctx context.Context, data []byte) error {
	var payload visitScheduledPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("error deserializando visit_scheduled: %w", err)
	}

	// Actualizar propuesta a ACCEPTED
	proposal, err := s.proposalRepo.FindByID(ctx, payload.ProposalID)
	if err != nil || proposal == nil {
		log.Printf("[CHAT NATS] Propuesta %s no encontrada, saltando\n", payload.ProposalID)
		return nil
	}
	_ = proposal.Accept()
	if err := s.proposalRepo.Update(ctx, proposal); err != nil {
		return err
	}

	// Inyectar mensaje de sistema en el chat
	sysMsg := domain.NewSystemMessage(
		payload.ConversationID,
		fmt.Sprintf("✅ Visita confirmada para el %s", payload.ScheduledAt),
		map[string]string{
			"visit_proposal_id": payload.ProposalID,
			"status":            "ACCEPTED",
			"scheduled_at":      payload.ScheduledAt,
		},
	)
	if err := s.msgRepo.Save(ctx, sysMsg); err != nil {
		return err
	}

	// Broadcast WebSocket
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"type":              "VISIT_CONFIRMED",
		"visit_proposal_id": payload.ProposalID,
		"scheduled_at":      payload.ScheduledAt,
		"status":            "ACCEPTED",
	})
	s.hub.Broadcast(payload.ConversationID, wsPayload)

	log.Printf("[CHAT] Visita %s confirmada, mensaje inyectado en conversación %s\n",
		payload.ProposalID, payload.ConversationID)
	return nil
}

func (s *CRMEventSubscriber) handleVisitRejected(ctx context.Context, data []byte) error {
	var payload visitRejectedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("error deserializando visit_rejected: %w", err)
	}

	proposal, err := s.proposalRepo.FindByID(ctx, payload.ProposalID)
	if err != nil || proposal == nil {
		log.Printf("[CHAT NATS] Propuesta %s no encontrada, saltando\n", payload.ProposalID)
		return nil
	}
	_ = proposal.Reject()
	if err := s.proposalRepo.Update(ctx, proposal); err != nil {
		return err
	}

	sysMsg := domain.NewSystemMessage(
		payload.ConversationID,
		"❌ El propietario rechazó la propuesta de visita.",
		map[string]string{
			"visit_proposal_id": payload.ProposalID,
			"status":            "REJECTED",
			"reason":            payload.Reason,
		},
	)
	if err := s.msgRepo.Save(ctx, sysMsg); err != nil {
		return err
	}

	wsPayload, _ := json.Marshal(map[string]interface{}{
		"type":              "VISIT_REJECTED",
		"visit_proposal_id": payload.ProposalID,
		"status":            "REJECTED",
		"reason":            payload.Reason,
	})
	s.hub.Broadcast(payload.ConversationID, wsPayload)

	log.Printf("[CHAT] Visita %s rechazada, mensaje inyectado en conversación %s\n",
		payload.ProposalID, payload.ConversationID)
	return nil
}
