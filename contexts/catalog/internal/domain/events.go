package domain

import (
	"crypto/rand"
	"fmt"
	"inmo.platform/shared/pkg/ddd"
)

// PropertySnapshot contiene la copia mínima de datos que Contratos necesita para
// calcular precios y validar restricciones de reserva sin llamar a Catálogo en tiempo real.
type PropertySnapshot struct {
	OwnerID         string        `json:"owner_id"`
	OperationType   string        `json:"operation_type"`
	NightPrice      float64       `json:"night_price"`
	CleaningFee     float64       `json:"cleaning_fee"`
	SecurityDeposit float64       `json:"security_deposit"`
	MinNights       int           `json:"min_nights"`
	MaxNights       int           `json:"max_nights"`
	CheckInTime     string        `json:"check_in_time"`
	CheckOutTime    string        `json:"check_out_time"`
	PricingRules    []PricingRule `json:"pricing_rules"`
}

// PropertyPublished se dispara cuando una propiedad pasa a estar disponible en el catálogo.
type PropertyPublished struct {
	ddd.BaseDomainEvent
	OwnerID  string          `json:"owner_id"`
	Snapshot PropertySnapshot `json:"snapshot"`
}

func NewPropertyPublished(p *Property) PropertyPublished {
	tc := p.TempConfig()
	return PropertyPublished{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			p.ID(),
			"catalog.property.published",
		),
		OwnerID: p.OwnerID(),
		Snapshot: PropertySnapshot{
			OwnerID:         p.OwnerID(),
			OperationType:   string(p.OperationType()),
			NightPrice:      tc.NightPrice(),
			CleaningFee:     tc.CleaningFee(),
			SecurityDeposit: tc.SecurityDeposit(),
			MinNights:       tc.MinNights(),
			MaxNights:       tc.MaxNights(),
			CheckInTime:     tc.CheckInTime(),
			CheckOutTime:    tc.CheckOutTime(),
			PricingRules:    tc.PricingRules(),
		},
	}
}

// PropertyUpdated se dispara cuando el propietario modifica precio/amenities de la propiedad.
type PropertyUpdated struct {
	ddd.BaseDomainEvent
	Snapshot PropertySnapshot `json:"snapshot"`
}

func NewPropertyUpdated(p *Property) PropertyUpdated {
	tc := p.TempConfig()
	return PropertyUpdated{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			p.ID(),
			"catalog.property.updated",
		),
		Snapshot: PropertySnapshot{
			OwnerID:         p.OwnerID(),
			OperationType:   string(p.OperationType()),
			NightPrice:      tc.NightPrice(),
			CleaningFee:     tc.CleaningFee(),
			SecurityDeposit: tc.SecurityDeposit(),
			MinNights:       tc.MinNights(),
			MaxNights:       tc.MaxNights(),
			CheckInTime:     tc.CheckInTime(),
			CheckOutTime:    tc.CheckOutTime(),
			PricingRules:    tc.PricingRules(),
		},
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
