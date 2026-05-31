package domain

import (
	"errors"
	"time"
)

// TicketStatus representa el enum de estados del ciclo de vida de la incidencia
type TicketStatus string

const (
	StatusOpen       TicketStatus = "OPEN"
	StatusValidated  TicketStatus = "VALIDATED"
	StatusQuoted     TicketStatus = "QUOTED"
	StatusApproved   TicketStatus = "APPROVED"
	StatusInProgress TicketStatus = "IN_PROGRESS"
	StatusClosed     TicketStatus = "CLOSED"
)

// UrgencyLevel define la clasificación por urgencia del ticket
type UrgencyLevel string

const (
	UrgencyEmergency UrgencyLevel = "EMERGENCY"
	UrgencyUrgent    UrgencyLevel = "URGENT"
	UrgencyScheduled UrgencyLevel = "SCHEDULED"
)

// Errores específicos de Dominio (Invariantes)
var (
	ErrInvalidStatusTransition = errors.New("transición de estado no permitida")
	ErrProviderRequired        = errors.New("no se puede presupuestar un ticket sin un proveedor asignado")
	ErrInvalidQuoteAmount      = errors.New("el monto del presupuesto debe ser mayor a cero")
	ErrTicketAlreadyClosed     = errors.New("el ticket ya se encuentra cerrado")
	ErrEvidenceRequired        = errors.New("el cierre del ticket requiere una descripción y evidencia válida")
)

// Quote representa el presupuesto de la reparación (Value Object / Entidad Interna)
type Quote struct {
	Amount   float64
	Details  string
	QuotedAt time.Time
}

// Evidence representa la prueba de finalización del trabajo (Value Object / Entidad Interna)
type Evidence struct {
	Description string
	DocumentURL string // Link a la foto/comprobante en AWS S3
	ClosedAt    time.Time
}

// Ticket es nuestro Aggregate Root
type Ticket struct {
	ID          string
	PropertyID  string
	TenantID    string
	ProviderID  string // Se llena en la validación
	Description string
	Status      TicketStatus
	Urgency     UrgencyLevel
	Quote       *Quote    // Opcional hasta QUOTED
	Evidence    *Evidence // Opcional hasta CLOSED
	CreatedAt   time.Time
}

// NewTicket es el constructor de fábrica para el reporte inicial (Estado: OPEN)
func NewTicket(id, propertyID, tenantID, description string, urgency UrgencyLevel) *Ticket {
	return &Ticket{
		ID:          id,
		PropertyID:  propertyID,
		TenantID:    tenantID,
		Description: description,
		Status:      StatusOpen,
		Urgency:     urgency,
		CreatedAt:   time.Now(),
	}
}

// AssignProvider pasa el ticket a VALIDATED y le asigna el técnico encargado
func (t *Ticket) AssignProvider(providerID string) error {
	if t.Status != StatusOpen {
		return ErrInvalidStatusTransition
	}
	if providerID == "" {
		return errors.New("el ID del proveedor no puede estar vacío")
	}

	t.ProviderID = providerID
	t.Status = StatusValidated
	return nil
}

// SubmitQuote inyecta el presupuesto. Invoca la invariante de proveedor asignado.
func (t *Ticket) SubmitQuote(amount float64, details string) error {
	// Invariante de flujo de estado
	if t.Status != StatusValidated {
		return ErrInvalidStatusTransition
	}
	// Invariante de Negocio: Debe tener proveedor
	if t.ProviderID == "" {
		return ErrProviderRequired
	}
	if amount <= 0 {
		return ErrInvalidQuoteAmount
	}

	t.Quote = &Quote{
		Amount:   amount,
		Details:  details,
		QuotedAt: time.Now(),
	}
	t.Status = StatusQuoted
	return nil
}

// Approve avanza el ticket a APPROVED para habilitar el inicio de obra
func (t *Ticket) Approve() error {
	// Invariante de Negocio: Solo un ticket presupuestado puede aprobarse
	if t.Status != StatusQuoted {
		return ErrInvalidStatusTransition
	}

	t.Status = StatusApproved
	return nil
}

// StartWork pasa el estado a ejecución activa
func (t *Ticket) StartWork() error {
	if t.Status != StatusApproved {
		return ErrInvalidStatusTransition
	}

	t.Status = StatusInProgress
	return nil
}

// Close finaliza el ciclo exigiendo la evidencia física del arreglo
func (t *Ticket) Close(description, documentURL string) error {
	if t.Status != StatusInProgress {
		return ErrInvalidStatusTransition
	}
	if description == "" || documentURL == "" {
		return ErrEvidenceRequired
	}

	t.Evidence = &Evidence{
		Description: description,
		DocumentURL: documentURL,
		ClosedAt:    time.Now(),
	}
	t.Status = StatusClosed
	return nil
}
