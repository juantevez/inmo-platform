package domain

import "errors"

type ContractState string

const (
	StateDraft      ContractState = "DRAFT"
	StateActive     ContractState = "ACTIVE"
	StateRenewed    ContractState = "RENEWED"
	StateTerminated ContractState = "TERMINATED"
)

var (
	ErrInvalidStateTransition = errors.New("transición de estado no permitida para el contrato")
)

// CanTransitionTo valida las reglas de la máquina de estados de un contrato
func (s ContractState) CanTransitionTo(next ContractState) bool {
	switch s {
	case StateDraft:
		// Un borrador solo puede pasar a Activo (cuando se firma)
		return next == StateActive
	case StateActive:
		// Un contrato activo puede ser Renovado o Rescindido/Terminado
		return next == StateRenewed || next == StateTerminated
	case StateRenewed:
		// Un contrato ya renovado puede volver a rescindirse en el futuro
		return next == StateTerminated
	case StateTerminated:
		// Desde Terminado no se puede ir a ningún lado (es un estado final)
		return false
	default:
		return false
	}
}
