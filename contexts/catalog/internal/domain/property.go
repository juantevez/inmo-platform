package domain

import (
	"inmo-platform/shared/pkg/apperr"
	"inmo-platform/shared/pkg/ddd"
)

type PropertyState string

const (
	StateAvailable   PropertyState = "AVAILABLE"
	StateReserved    PropertyState = "RESERVED"
	StateClosed      PropertyState = "CLOSED"
	StateUnderRepair PropertyState = "UNDER_REPAIR"
)

// Property es la raíz del agregado para el contexto de Catálogo.
type Property struct {
	ddd.AggregateRoot
	id          string
	ownerID     string
	title       string
	description string
	price       Price
	location    Location
	state       PropertyState
}

// NewProperty es el constructor de fábrica que garantiza invariantes de creación.
func NewProperty(id, ownerID, title, description string, price Price, location Location) (*Property, error) {
	if id == "" || ownerID == "" {
		return nil, apperr.NewBadRequest("el ID de la propiedad y del propietario son obligatorios", nil)
	}
	if title == "" {
		return nil, apperr.NewBadRequest("el título de la publicación no puede estar vacío", nil)
	}

	p := &Property{
		id:          id,
		ownerID:     ownerID,
		title:       title,
		description: description,
		price:       price,
		location:    location,
		state:       StateAvailable, // Nace disponible por defecto
	}

	// Registramos el evento de publicación inicial
	p.RecordEvent(NewPropertyPublished(p.id, p.ownerID))

	return p, nil
}

// --- Máquina de Estados (Invariantes de Transición) ---

// Reserve pasa la propiedad a RESERVED solo si estaba AVAILABLE.
func (p *Property) Reserve() error {
	if p.state != StateAvailable {
		return apperr.NewPreconditionFailed("solo se puede reservar una propiedad que esté disponible", nil)
	}

	old := p.state
	p.state = StateReserved
	p.RecordEvent(NewPropertyStateChanged(p.id, old, p.state))
	return nil
}

// Close pasa la propiedad a CLOSED (alquilada/vendida) desde AVAILABLE o RESERVED.
func (p *Property) Close() error {
	if p.state == StateClosed || p.state == StateUnderRepair {
		return apperr.NewPreconditionFailed("no se puede cerrar una propiedad inactiva o en reparación", nil)
	}

	old := p.state
	p.state = StateClosed
	p.RecordEvent(NewPropertyStateChanged(p.id, old, p.state))
	return nil
}

// PutUnderRepair bloquea la propiedad para reparaciones.
func (p *Property) PutUnderRepair() error {
	if p.state == StateUnderRepair {
		return nil // Idempotente
	}

	old := p.state
	p.state = StateUnderRepair
	p.RecordEvent(NewPropertyStateChanged(p.id, old, p.state))
	return nil
}

// ReleaseRepair devuelve la propiedad al estado AVAILABLE tras terminar el mantenimiento.
func (p *Property) ReleaseRepair() error {
	if p.state != StateUnderRepair {
		return apperr.NewPreconditionFailed("la propiedad no se encuentra bajo reparación", nil)
	}

	old := p.state
	p.state = StateAvailable
	p.RecordEvent(NewPropertyStateChanged(p.id, old, p.state))
	return nil
}

// --- Getters limpios ---

func (p *Property) ID() string           { return p.id }
func (p *Property) OwnerID() string      { return p.ownerID }
func (p *Property) Title() string        { return p.title }
func (p *Property) Description() string  { return p.description }
func (p *Property) Price() Price         { return p.price }
func (p *Property) Location() Location   { return p.location }
func (p *Property) State() PropertyState { return p.state }
