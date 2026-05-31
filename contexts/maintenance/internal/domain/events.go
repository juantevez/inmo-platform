package domain

import "time"

// TicketApprovedEvent se dispara cuando los administradores aprueban el presupuesto
type TicketApprovedEvent struct {
	TicketID   string    `json:"ticket_id"`
	PropertyID string    `json:"property_id"`
	ApprovedAt time.Time `json:"approved_at"`
}

// TicketClosedEvent se dispara al concluir satisfactoriamente la reparación con evidencia
type TicketClosedEvent struct {
	TicketID    string    `json:"ticket_id"`
	PropertyID  string    `json:"property_id"`
	ProviderID  string    `json:"provider_id"`
	Cost        float64   `json:"cost"`
	Description string    `json:"description"`
	ClosedAt    time.Time `json:"closed_at"`
}
