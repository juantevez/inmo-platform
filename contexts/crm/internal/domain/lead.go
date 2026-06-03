package domain

import (
	"errors"
	"time"
)

// LeadState mapea directamente al tipo VARCHAR(50) de tu columna 'state'
type LeadState string

const (
	StateNew            LeadState = "NEW"
	StateContacted      LeadState = "CONTACTED"
	StateVisitScheduled LeadState = "VISIT_SCHEDULED"
	StateClosed         LeadState = "CLOSED"
)

// Errores de dominio (Invariantes)
var (
	ErrInvalidContact    = errors.New("el lead debe tener al menos un email o teléfono válido")
	ErrInvalidTransition = errors.New("transición de estado no permitida en un lead cerrado")
)

type Lead struct {
	ID         string
	PropertyID string    // Mapea a property_id
	ClientName string    // Mapea a client_name
	Email      string    // Mapea a email
	Phone      string    // Mapea a phone
	State      LeadState // Mapea a state
	CreatedAt  time.Time // Mapea a created_at
	UpdatedAt  time.Time // Mapea a updated_at
}

// NewLead es el constructor del Agregado Raíz. Asegura las invariantes de creación.
func NewLead(id, propertyID, clientName, email, phone string) (*Lead, error) {
	// Invariante 1: Todo lead debe referenciar un medio de contacto (email o teléfono)
	if email == "" && phone == "" {
		return nil, ErrInvalidContact
	}

	return &Lead{
		ID:         id,
		PropertyID: propertyID,
		ClientName: clientName,
		Email:      email,
		Phone:      phone,
		State:      StateNew, // Inicia siempre en estado NUEVO
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// TransitionTo permite avanzar el estado del Lead de forma segura
func (l *Lead) TransitionTo(newState LeadState) error {
	// Invariante: Un lead cerrado ya no puede cambiar de estado
	if l.State == StateClosed {
		return ErrInvalidTransition
	}

	l.State = newState
	l.UpdatedAt = time.Now()
	return nil
}
