package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// reservationNotificationSubscriber escucha eventos de reserva y publica
// mensajes de sistema en la conversación de chat correspondiente.
//
// Flujo:
//   contracts.reservation.confirmed  → mensaje "✅ Tu reserva fue aprobada"
//   contracts.reservation.cancelled  → mensaje "❌ Tu reserva fue rechazada"
//
// El recordatorio de 24hs se maneja por separado en el ReminderScheduler.

type ReservationNotificationSubscriber struct {
	js      jetstream.JetStream
	chatURL string // base URL del gateway, ej: http://localhost:8000
	client  *http.Client
}

func NewReservationNotificationSubscriber(js jetstream.JetStream) *ReservationNotificationSubscriber {
	chatURL := os.Getenv("GATEWAY_URL")
	if chatURL == "" {
		chatURL = "http://localhost:8000"
	}
	return &ReservationNotificationSubscriber{
		js:      js,
		chatURL: chatURL,
		client:  &http.Client{Timeout: 8 * time.Second},
	}
}

// StartConsume bloquea hasta que ctx sea cancelado.
func (s *ReservationNotificationSubscriber) StartConsume(ctx context.Context) error {
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "contracts", jetstream.ConsumerConfig{
		Durable:        "contracts-reservation-notifications",
		FilterSubjects: []string{"contracts.reservation.confirmed", "contracts.reservation.cancelled"},
		AckPolicy:      jetstream.AckExplicitPolicy,
		// Reintentar hasta 5 veces con backoff exponencial antes de descartar
		MaxDeliver: 5,
		BackOff:    []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second, 120 * time.Second},
	})
	if err != nil {
		return fmt.Errorf("[NOTIF] error al crear consumidor de notificaciones: %w", err)
	}

	iter, err := cons.Messages()
	if err != nil {
		return fmt.Errorf("[NOTIF] error al obtener iterador: %w", err)
	}
	defer iter.Stop()

	go func() {
		<-ctx.Done()
		iter.Stop()
	}()

	log.Println("[NOTIF] Escuchando eventos de reserva para notificaciones al inquilino...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil // shutdown limpio
			}
			return fmt.Errorf("[NOTIF] error al iterar mensajes: %w", err)
		}

		if err := s.processMessage(ctx, msg); err != nil {
			log.Printf("[NOTIF ERROR] %v — se reintentará automáticamente\n", err)
			// NAK para que NATS reintente con backoff
			_ = msg.Nak()
			continue
		}

		_ = msg.Ack()
	}
}

// ── Payloads de eventos entrantes ────────────────────────────────────────────

type reservationConfirmedPayload struct {
	ReservationID string  `json:"aggregate_id"` // viene de BaseDomainEvent
	PropertyID    string  `json:"property_id"`
	TenantID      string  `json:"tenant_id"`
	OwnerID       string  `json:"owner_id"`
	CheckInDate   string  `json:"check_in_date"`
	CheckOutDate  string  `json:"check_out_date"`
	TotalAmount   float64 `json:"total_amount"`
}

type reservationCancelledPayload struct {
	ReservationID string `json:"aggregate_id"`
	PropertyID    string `json:"property_id"`
}

// ── Procesamiento ─────────────────────────────────────────────────────────────

func (s *ReservationNotificationSubscriber) processMessage(ctx context.Context, msg jetstream.Msg) error {
	subject := msg.Subject()

	switch subject {
	case "contracts.reservation.confirmed":
		var p reservationConfirmedPayload
		if err := json.Unmarshal(msg.Data(), &p); err != nil {
			return fmt.Errorf("error al deserializar confirmed: %w", err)
		}
		return s.notifyConfirmed(ctx, p)

	case "contracts.reservation.cancelled":
		var p reservationCancelledPayload
		if err := json.Unmarshal(msg.Data(), &p); err != nil {
			return fmt.Errorf("error al deserializar cancelled: %w", err)
		}
		return s.notifyCancelled(ctx, p)

	default:
		log.Printf("[NOTIF] Subject inesperado ignorado: %s\n", subject)
		return nil
	}
}

func (s *ReservationNotificationSubscriber) notifyConfirmed(ctx context.Context, p reservationConfirmedPayload) error {
	text := fmt.Sprintf(
		"✅ *Tu reserva fue confirmada por el propietario.*\n\n"+
			"📅 Check-in: %s\n"+
			"📅 Check-out: %s\n"+
			"💰 Total acordado: $%.2f\n\n"+
			"¡Nos vemos pronto! Podés comunicarte con el propietario desde este chat.",
		p.CheckInDate, p.CheckOutDate, p.TotalAmount,
	)
	return s.postSystemMessage(ctx, p.PropertyID, p.TenantID, p.OwnerID, text)
}

func (s *ReservationNotificationSubscriber) notifyCancelled(ctx context.Context, p reservationCancelledPayload) error {
	text := "❌ *El propietario no pudo confirmar tu reserva para estas fechas.*\n\n" +
		"Podés escribirle directamente para consultar disponibilidad o elegir otras fechas."
	// Para el cancel solo tenemos PropertyID; el chat service resuelve la conversación por property
	return s.postSystemMessage(ctx, p.PropertyID, "", "", text)
}

// ── Comunicación con el Chat Service vía Gateway ──────────────────────────────

// postSystemMessage encuentra la conversación entre tenant y owner para esa propiedad
// y publica un mensaje de tipo "system" en ella.
func (s *ReservationNotificationSubscriber) postSystemMessage(
	ctx context.Context,
	propertyID, tenantID, ownerID, text string,
) error {
	// 1. Obtener (o crear idempotentemente) la conversación
	chatID, err := s.resolveChat(ctx, propertyID, tenantID, ownerID)
	if err != nil {
		return fmt.Errorf("no se pudo resolver chat para propiedad %s: %w", propertyID, err)
	}

	// 2. Publicar el mensaje de sistema
	body, _ := json.Marshal(map[string]interface{}{
		"body":         text,
		"message_type": "system", // el chat service diferencia mensajes de sistema de los de usuario
	})

	url := fmt.Sprintf("%s/api/v1/chats/%s/messages", s.chatURL, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Usamos una service-account interna; el gateway no requiere JWT para mensajes de sistema
	req.Header.Set("X-Service-Token", os.Getenv("SERVICE_TOKEN"))
	req.Header.Set("X-User-Id", "system") // el chat lo identifica como remitente automático

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("error HTTP al postear mensaje: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("chat service respondió %d al postear mensaje de sistema", resp.StatusCode)
	}

	log.Printf("[NOTIF] Mensaje de sistema enviado al chat %s (propiedad %s)\n", chatID, propertyID)
	return nil
}

// resolveChat llama a POST /api/v1/chats (idempotente) para obtener el chat_id.
// Si la conversación ya existe, el chat service la devuelve; si no, la crea.
func (s *ReservationNotificationSubscriber) resolveChat(
	ctx context.Context,
	propertyID, tenantID, ownerID string,
) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"property_id":   propertyID,
		"advertiser_id": ownerID,
	})

	url := fmt.Sprintf("%s/api/v1/chats", s.chatURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", os.Getenv("SERVICE_TOKEN"))
	req.Header.Set("X-User-Id", tenantID) // actúa en nombre del inquilino

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error al resolver chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat service respondió %d al resolver conversación", resp.StatusCode)
	}

	var result struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"` // alias que usan algunos clientes
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta del chat: %w", err)
	}

	chatID := result.ID
	if chatID == "" {
		chatID = result.ConversationID
	}
	if chatID == "" {
		return "", fmt.Errorf("chat service no devolvió un ID de conversación")
	}
	return chatID, nil
}
