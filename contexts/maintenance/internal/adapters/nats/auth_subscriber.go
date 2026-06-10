package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

// authUserCreatedEvent mapea el payload de auth.user.created.
// Solo extraemos los campos que necesitamos — el verification_token lo ignoramos.
//
// Estructura del evento emitido por register_user.go:
//
//	{
//	  "event_id":   "...",
//	  "event_name": "auth.user.created",
//	  "user_id":    "...",
//	  "timestamp":  "...",
//	  "payload": {
//	    "email":              "...",
//	    "role":               "INQUILINO",
//	    "verification_token": "..."
//	  }
//	}
type authUserCreatedEvent struct {
	EventID   string                 `json:"event_id"`
	EventName string                 `json:"event_name"`
	UserID    string                 `json:"user_id"`
	Payload   map[string]interface{} `json:"payload"`
}

// AuthSubscriber consume eventos de auth para mantener la proyección
// local de inquilinos en inmo_maintenance_db.
//
// Subjects que consume:
//   - auth.user.created → si role == "INQUILINO", Upsert en inquilino_projections
//
// Consumer durable: "maintenance-auth-sync"
// Stream esperado:  "auth" (debe crearse en auth-identity o aquí al arrancar)
type AuthSubscriber struct {
	js                      jetstream.JetStream
	inquilinoProjectionRepo ports.InquilinoProjectionRepository
}

func NewAuthSubscriber(js jetstream.JetStream, repo ports.InquilinoProjectionRepository) *AuthSubscriber {
	return &AuthSubscriber{
		js:                      js,
		inquilinoProjectionRepo: repo,
	}
}

// StartConsume inicia el consumidor durable y bloquea hasta que el contexto se cancele.
// Debe correrse en una goroutine separada desde main.go.
func (s *AuthSubscriber) StartConsume(ctx context.Context) error {
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "auth", jetstream.ConsumerConfig{
		Durable:       "maintenance-auth-sync",
		FilterSubject: "auth.user.created",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("[MAINTENANCE AUTH SUB] error al crear consumidor durable: %w", err)
	}

	iter, err := cons.Messages()
	if err != nil {
		return fmt.Errorf("[MAINTENANCE AUTH SUB] error al obtener iterador: %w", err)
	}
	defer iter.Stop()

	go func() {
		<-ctx.Done()
		iter.Stop()
	}()

	log.Println("[MAINTENANCE AUTH SUB] Escuchando auth.user.created...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				log.Println("[MAINTENANCE AUTH SUB] Contexto cancelado, deteniendo subscriber.")
				return nil
			}
			return fmt.Errorf("[MAINTENANCE AUTH SUB] error al iterar mensajes: %w", err)
		}

		if err := s.handleUserCreated(ctx, msg.Data()); err != nil {
			// No hacemos Ack — NATS reintentará el mensaje
			log.Printf("[MAINTENANCE AUTH SUB ERROR] %v", err)
			continue
		}

		_ = msg.Ack()
	}
}

// handleUserCreated procesa auth.user.created.
// Solo actúa si el role del payload es "INQUILINO" — ignora PROPIETARIO, AGENTE, etc.
func (s *AuthSubscriber) handleUserCreated(ctx context.Context, data []byte) error {
	var event authUserCreatedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("error al deserializar auth.user.created: %w", err)
	}

	// Extraer role del payload
	role, _ := event.Payload["role"].(string)
	if role != "INQUILINO" {
		// Ignoramos silenciosamente — PROPIETARIO, AGENTE, PROVEEDOR no necesitan proyección acá
		log.Printf("[MAINTENANCE AUTH SUB] Ignorando user_id=%s role=%s", event.UserID, role)
		return nil
	}

	// Extraer email del payload
	email, _ := event.Payload["email"].(string)
	if email == "" || event.UserID == "" {
		return fmt.Errorf("evento auth.user.created inválido: user_id o email vacíos (event_id=%s)", event.EventID)
	}

	projection := domain.NewInquilinoProjection(event.UserID, email)

	if err := s.inquilinoProjectionRepo.Upsert(ctx, projection); err != nil {
		return fmt.Errorf("error al hacer Upsert de inquilino user_id=%s: %w", event.UserID, err)
	}

	log.Printf("[MAINTENANCE AUTH SUB] Inquilino sincronizado: user_id=%s email=%s", event.UserID, email)
	return nil
}
