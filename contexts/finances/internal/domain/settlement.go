package domain

import (
	"errors"
	"fmt"
	"time"
)

// Errores de Dominio para el control de Invariantes
var (
	ErrSettlementNotOpen       = errors.New("la liquidación no está abierta; no se pueden modificar sus conceptos")
	ErrInvalidConceptAmount    = errors.New("el monto del concepto debe ser mayor a cero")
	ErrConceptNotFound         = errors.New("el concepto especificado no existe en esta liquidación")
	ErrInvalidStatusTransition = errors.New("transición de estado no permitida para la liquidación")
	ErrEmptyConceptDescription = errors.New("la descripción del concepto no puede estar vacía")
)

// SettlementStatus representa el estado de la liquidación (Máquina de estados)
type SettlementStatus string

const (
	StatusOpen   SettlementStatus = "OPEN"
	StatusClosed SettlementStatus = "CLOSED"
	StatusPaid   SettlementStatus = "PAID"
)

// ConceptType define fuertemente el rubro del concepto cargado
type ConceptType string

const (
	TypeRent       ConceptType = "RENT"
	TypeTax        ConceptType = "TAX"      // ABL, Inmobiliario, etc.
	TypeExpenses   ConceptType = "EXPENSES" // Expensas de la propiedad
	TypeAdjustment ConceptType = "ADJUSTMENT"

	// 🔌 Servicios abiertos detallados (Opción 2)
	TypeElectricity ConceptType = "UTILITY_ELECTRICITY"
	TypeWater       ConceptType = "UTILITY_WATER"
	TypeGas         ConceptType = "UTILITY_GAS"
	TypeInternet    ConceptType = "UTILITY_INTERNET"
	TypeCableTV     ConceptType = "UTILITY_CABLE_TV"
)

// SettlementConcept representa una Entidad interna del Agregado (Detalle de la liquidación)
type SettlementConcept struct {
	id          string
	description string
	conceptType ConceptType
	amount      float64
}

// Getters públicos de la Entidad Concepto
func (c *SettlementConcept) ID() string          { return c.id }
func (c *SettlementConcept) Description() string { return c.description }
func (c *SettlementConcept) Type() ConceptType   { return c.conceptType }
func (c *SettlementConcept) Amount() float64     { return c.amount }

// Settlement es la Raíz del Agregado (Aggregate Root)
type Settlement struct {
	id         string
	contractID string // UUID referencial al Bounded Context de Contratos
	period     string // Formato estricto "YYYY-MM" (e.g., "2026-05")
	status     SettlementStatus
	concepts   []*SettlementConcept
	createdAt  time.Time
	closedAt   *time.Time
}

// NewSettlement es la Fábrica (Factory) para iniciar una liquidación mensual limpia
func NewSettlement(id string, contractID string, period string) (*Settlement, error) {
	if id == "" || contractID == "" || period == "" {
		return nil, errors.New("el ID de liquidación, ID de contrato y el período son obligatorios")
	}

	return &Settlement{
		id:         id,
		contractID: contractID,
		period:     period,
		status:     StatusOpen,
		concepts:   make([]*SettlementConcept, 0),
		createdAt:  time.Now(),
	}, nil // ◄ AGREGADO EL 'nil' AQUÍ PARA CUMPLIR LA FIRMA
}

// ReconstituteSettlement se usa exclusivamente en la capa de infraestructura (Repohidratación)
// Permite armar el objeto desde la BD salteando las reglas de inicialización de la Fábrica
func ReconstituteSettlement(id, contractID, period string, status SettlementStatus, concepts []*SettlementConcept, createdAt time.Time, closedAt *time.Time) *Settlement {
	return &Settlement{
		id:         id,
		contractID: contractID,
		period:     period,
		status:     status,
		concepts:   concepts,
		createdAt:  createdAt,
		closedAt:   closedAt,
	}
}

// ReconstituteConcept permite a la infraestructura rehidratar conceptos internos
func ReconstituteConcept(id, description string, conceptType ConceptType, amount float64) *SettlementConcept {
	return &SettlementConcept{
		id:          id,
		description: description,
		conceptType: conceptType,
		amount:      amount,
	}
}

// AddConcept añade un nuevo concepto respetando la invariante fundamental de estado
func (s *Settlement) AddConcept(id string, desc string, cType ConceptType, amount float64) error {
	// Invariante: Solo se pueden agregar conceptos mientras la liquidación está Abierta
	if s.status != StatusOpen {
		return ErrSettlementNotOpen
	}

	if amount <= 0 {
		return ErrInvalidConceptAmount
	}
	if desc == "" {
		return ErrEmptyConceptDescription
	}

	concept := &SettlementConcept{
		id:          id,
		description: desc,
		conceptType: cType,
		amount:      amount,
	}

	s.concepts = append(s.concepts, concept)
	return nil
}

// RemoveConcept remueve un concepto de la lista si sigue abierta
func (s *Settlement) RemoveConcept(conceptID string) error {
	// Invariante: No se puede alterar la liquidación si no está abierta
	if s.status != StatusOpen {
		return ErrSettlementNotOpen
	}

	for i, c := range s.concepts {
		if c.id == conceptID {
			// Slicing rápido para eliminar el elemento manteniendo punteros limpios
			s.concepts = append(s.concepts[:i], s.concepts[i+1:]...)
			return nil
		}
	}

	return ErrConceptNotFound
}

// Total calcula de forma derivada el total exacto sumando sus conceptos actuales
func (s *Settlement) Total() float64 {
	var total float64
	for _, c := range s.concepts {
		total += c.amount
	}
	return total
}

// Close cierra la liquidación. Bloquea futuras modificaciones y calcula el total de emisión.
func (s *Settlement) Close() error {
	if s.status != StatusOpen {
		return fmt.Errorf("%w: solo se puede cerrar una liquidación abierta (estado actual: %s)", ErrInvalidStatusTransition, s.status)
	}

	if len(s.concepts) == 0 {
		return errors.New("no se puede cerrar una liquidación que no tiene conceptos cargados")
	}

	now := time.Now()
	s.status = StatusClosed
	s.closedAt = &now

	return nil
}

// MarkAsPaid pasa la liquidación a su estado final de cobro/pago efectivo
func (s *Settlement) MarkAsPaid() error {
	if s.status != StatusClosed {
		return fmt.Errorf("%w: solo se puede pagar una liquidación que esté en estado CLOSED (estado actual: %s)", ErrInvalidStatusTransition, s.status)
	}

	s.status = StatusPaid
	return nil
}

// Getters del Aggregate Root (Campos inmutables desde afuera del paquete)
func (s *Settlement) ID() string                     { return s.id }
func (s *Settlement) ContractID() string             { return s.contractID }
func (s *Settlement) Period() string                 { return s.period }
func (s *Settlement) Status() SettlementStatus       { return s.status }
func (s *Settlement) Concepts() []*SettlementConcept { return s.concepts }
func (s *Settlement) CreatedAt() time.Time           { return s.createdAt }
func (s *Settlement) ClosedAt() *time.Time           { return s.closedAt }
