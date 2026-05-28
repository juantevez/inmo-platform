package domain

import (
	"crypto/rand"
	"fmt"
	"inmo-platform/shared/pkg/ddd"
)

// PropertyPublished se dispara cuando una propiedad pasa a estar disponible en el catálogo.
type PropertyPublished struct {
	ddd.BaseDomainEvent
	OwnerID string `json:"owner_id"`
}

func NewPropertyPublished(propertyID, ownerID string) PropertyPublished {
	return PropertyPublished{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			propertyID,
			"catalog.property.published",
		),
		OwnerID: ownerID,
	}
}

// PropertyStateChanged se dispara ante cualquier transición en la máquina de estados.
type PropertyStateChanged struct {
	ddd.BaseDomainEvent
	OldState PropertyState `json:"old_state"`
	NewState PropertyState `json:"new_state"`
}

func NewPropertyStateChanged(propertyID string, oldState, newState PropertyState) PropertyStateChanged {
	return PropertyStateChanged{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			propertyID,
			"catalog.property.state_changed",
		),
		OldState: oldState,
		NewState: newState,
	}
}

// Helper rápido para generar IDs de eventos sin arrastrar dependencias externas pesadas aún
func nextUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
