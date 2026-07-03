package domain

import (
	"errors"
	"time"

	"inmo.platform/shared/pkg/apperr"
)

// LeadState mapea directamente al tipo VARCHAR(50) de tu columna 'state'
type LeadState string

const (
	StateNew            LeadState = "NEW"
	StateContacted      LeadState = "CONTACTED"
	StateVisitScheduled LeadState = "VISIT_SCHEDULED"
	StateClosed         LeadState = "CLOSED"
)

// ErrInvalidTransition señala una transición de estado no permitida (p.ej. lead
// cerrado). Se envuelve en un apperr.PreconditionFailed en la capa de aplicación
// (ver contracts/internal/application/activate_contract.go para el mismo patrón).
var ErrInvalidTransition = errors.New("transición de estado no permitida para el lead")

type Lead struct {
	ID               string
	PropertyID       string     // Mapea a property_id
	ClientName       string     // Mapea a client_name
	Email            string     // Mapea a email
	Phone            string     // Mapea a phone
	State            LeadState  // Mapea a state
	VisitScheduledAt *time.Time // Mapea a visit_scheduled_at (nil hasta que se agenda)
	CreatedAt        time.Time  // Mapea a created_at
	UpdatedAt        time.Time  // Mapea a updated_at
}

// NewLead es el constructor del Agregado Raíz. Asegura las invariantes de creación.
func NewLead(id, propertyID, clientName, email, phone string) (*Lead, error) {
	if id == "" {
		return nil, apperr.NewBadRequest("id del lead es obligatorio", nil)
	}
	// Invariante: todo lead debe referenciar una propiedad
	if propertyID == "" {
		return nil, apperr.NewBadRequest("el lead debe referenciar una propiedad", nil)
	}
	// Invariante: todo lead debe tener un medio de contacto (email o teléfono)
	if email == "" && phone == "" {
		return nil, apperr.NewBadRequest("el lead debe tener al menos un email o teléfono válido", nil)
	}

	now := time.Now()
	return &Lead{
		ID:         id,
		PropertyID: propertyID,
		ClientName: clientName,
		Email:      email,
		Phone:      phone,
		State:      StateNew, // Inicia siempre en estado NUEVO
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// MarkContacted registra que un agente hizo el primer contacto con el lead.
// Solo procede desde el estado NEW.
func (l *Lead) MarkContacted() error {
	if l.State != StateNew {
		return ErrInvalidTransition
	}
	l.State = StateContacted
	l.UpdatedAt = time.Now()
	return nil
}

// ScheduleVisit agenda una visita para una fecha futura. Solo procede desde
// CONTACTED. Invariante: la visita solo puede agendarse a futuro.
func (l *Lead) ScheduleVisit(visitAt time.Time) error {
	if l.State != StateContacted {
		return ErrInvalidTransition
	}
	if !visitAt.After(time.Now()) {
		return apperr.NewBadRequest("la visita solo puede agendarse para una fecha futura", nil)
	}

	l.State = StateVisitScheduled
	l.VisitScheduledAt = &visitAt
	l.UpdatedAt = time.Now()
	return nil
}

// Close cierra el lead. Procede desde cualquier estado que no sea ya CLOSED —
// un lead puede abandonarse/cerrarse en cualquier etapa del funnel.
func (l *Lead) Close() error {
	if l.State == StateClosed {
		return ErrInvalidTransition
	}
	l.State = StateClosed
	l.UpdatedAt = time.Now()
	return nil
}
