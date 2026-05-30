package ports

import (
	"context"
)

// ContractService define las operaciones necesarias que requerimos del Bounded Context de Contratos
type ContractService interface {
	// IsContractActive verifica si el contrato existe y está apto para recibir cargos
	IsContractActive(ctx context.Context, contractID string) (bool, error)
}

// EventDispatcher define el puerto para publicar eventos de dominio (hacia NATS o Kafka)
type EventDispatcher interface {
	// PublishSettlementClosed notifica al resto del sistema que la liquidación se cerró y emitió sus totales
	PublishSettlementClosed(ctx context.Context, settlementID string) error
}
