package domain

import (
	"fmt"
	"inmo.platform/shared/pkg/ddd"
	"time"
)

// Contract es el Aggregate Root del contexto de Contratos
type Contract struct {
	ddd.AggregateRoot
	id               string
	propertyID       string
	tenantID         string
	ownerID          string
	rentAmount       RentAmount
	timeline         Timeline
	index            AdjustmentIndex
	adjustmentPeriod AdjustmentPeriod
	state            ContractState
	createdAt        time.Time
}

// NewContract es el Factory Method para inicializar un contrato en estado DRAFT (Borrador)
func NewContract(id, propertyID, tenantID, ownerID string, rent RentAmount, timeline Timeline, idx AdjustmentIndex, period AdjustmentPeriod) *Contract {
	return &Contract{
		id:               id,
		propertyID:       propertyID,
		tenantID:         tenantID,
		ownerID:          ownerID,
		rentAmount:       rent,
		timeline:         timeline,
		index:            idx,
		adjustmentPeriod: period,
		state:            StateDraft, // Nace como borrador siempre
		createdAt:        time.Now(),
	}
}

// Getters de acceso seguro
func (c *Contract) ID() string                         { return c.id }
func (c *Contract) PropertyID() string                 { return c.propertyID }
func (c *Contract) TenantID() string                   { return c.tenantID }
func (c *Contract) OwnerID() string                    { return c.ownerID }
func (c *Contract) RentAmount() RentAmount             { return c.rentAmount }
func (c *Contract) Timeline() Timeline                 { return c.timeline }
func (c *Contract) Index() AdjustmentIndex             { return c.index }
func (c *Contract) AdjustmentPeriod() AdjustmentPeriod { return c.adjustmentPeriod }
func (c *Contract) State() ContractState               { return c.state }

// Activate ejecuta la regla de negocio de la firma del contrato y dispara el evento de integración
func (c *Contract) Activate() error {
	if !c.state.CanTransitionTo(StateActive) {
		return fmt.Errorf("%w: no se puede activar un contrato en estado %s", ErrInvalidStateTransition, c.state)
	}

	c.state = StateActive

	// Aquí es donde registramos el evento de dominio. Al activarse el contrato,
	// el sistema distribuido debe enterarse para sacar la propiedad del Catálogo automáticamente.
	c.RecordEvent(NewContractActivated(c.id, c.propertyID))

	return nil
}

// ApplyAdjustment aplica un coeficiente sobre el valor actual del alquiler (caso de uso del worker indexador)
func (c *Contract) ApplyAdjustment(coefficient float64) error {
	if c.state != StateActive {
		return fmt.Errorf("no se puede ajustar el valor de un contrato que no está ACTIVO (estado actual: %s)", c.state)
	}
	if coefficient <= 0 {
		return fmt.Errorf("el coeficiente de ajuste debe ser mayor a cero")
	}

	newAmount := c.rentAmount.amount * coefficient
	c.rentAmount.amount = newAmount

	// Podríamos emitir un evento 'contracts.contract.adjusted' si finanzas o facturación lo necesitasen
	return nil
}

// RehydrateContract permite a la capa de infraestructura (Postgres) reconstruir un agregado existente desde la BD
func RehydrateContract(id, propertyID, tenantID, ownerID string, rent RentAmount, timeline Timeline, idx AdjustmentIndex, period AdjustmentPeriod, state ContractState, createdAt time.Time) *Contract {
	return &Contract{
		id:               id,
		propertyID:       propertyID,
		tenantID:         tenantID,
		ownerID:          ownerID,
		rentAmount:       rent,
		timeline:         timeline,
		index:            idx,
		adjustmentPeriod: period,
		state:            state,
		createdAt:        createdAt,
	}
}

// De paso agregamos este getter rápido que nos pide el repositorio
func (c *Contract) CreatedAt() time.Time { return c.createdAt }
