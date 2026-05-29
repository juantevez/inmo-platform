package domain

import (
	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

type LeadState string

const (
	StateNew            LeadState = "NEW"
	StateContacted      LeadState = "CONTACTED"
	StateVisitScheduled LeadState = "VISIT_SCHEDULED"
	StateClosed         LeadState = "CLOSED"
)

// Lead es la raíz del agregado para el contexto de CRM.
type Lead struct {
	ddd.AggregateRoot
	id         string
	propertyID string
	clientName string
	email      string
	phone      string
	state      LeadState
}

// NewLead es la fábrica que garantiza que todo interés esté ligado a una propiedad y un medio de contacto.
func NewLead(id, propertyID, clientName, email, phone string) (*Lead, error) {
	if id == "" || propertyID == "" {
		return nil, apperr.NewBadRequest("el ID del lead y de la propiedad son obligatorios", nil)
	}
	if clientName == "" {
		return nil, apperr.NewBadRequest("el nombre del cliente es obligatorio", nil)
	}
	if email == "" && phone == "" {
		return nil, apperr.NewBadRequest("debe proveer al menos un medio de contacto (email o teléfono)", nil)
	}

	return &Lead{
		id:         id,
		propertyID: propertyID,
		clientName: clientName,
		email:      email,
		phone:      phone,
		state:      StateNew,
	}, nil
}

// --- Mutadores de Estado ---

func (l *Lead) MarkAsContacted() {
	if l.state == StateNew {
		l.state = StateContacted
	}
}

// Getters
func (l *Lead) ID() string         { return l.id }
func (l *Lead) PropertyID() string { return l.propertyID }
func (l *Lead) ClientName() string { return l.clientName }
func (l *Lead) Email() string      { return l.email }
func (l *Lead) Phone() string      { return l.phone }
func (l *Lead) State() LeadState   { return l.state }
