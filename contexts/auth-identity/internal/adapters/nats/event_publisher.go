package ports

import (
	"context"
	"time"
)

// AuthEvent es la estructura de datos estandarizada para el transporte de eventos de identidad.
// Centraliza la data que leerán componentes como tu 'whatsapp-bot-service' o servicios de mail.
type AuthEvent struct {
	EventID   string                 `json:"event_id"`
	Name      string                 `json:"event_name"` // Ej: "auth.user.created", "auth.user.logged_in"
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"` // Data flexible (email, tokens, metatada de red, etc)
}

type EventPublisher interface {
	// PublishEvent despacha un evento de identidad de forma asincrónica al bus (NATS JetStream)
	PublishEvent(ctx context.Context, event AuthEvent) error
}
