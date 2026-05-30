package ports

import (
	"context"
	"time"
)

// AuthEvent es una estructura genérica para simplificar el transporte de eventos de identidad
type AuthEvent struct {
	EventID   string                 `json:"event_id"`
	Name      string                 `json:"event_name"` // Ej: "auth.user.created", "auth.user.logged_in"
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

type EventPublisher interface {
	// PublishEvent despacha un evento de autenticación/identidad al bus de NATS (UC-01/03/10)
	PublishEvent(ctx context.Context, event AuthEvent) error
}
