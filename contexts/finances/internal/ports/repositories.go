package ports

import (
	"context"

	"inmo.platform/contexts/finances/internal/domain"
)

type SettlementRepository interface {
	// Save inserta una nueva liquidación abierta por primera vez
	Save(ctx context.Context, settlement *domain.Settlement) error

	// Update actualiza el estado de la liquidación o guarda/remueve conceptos mutados
	Update(ctx context.Context, settlement *domain.Settlement) error

	// FindByID recupera el agregado completo (hidratado con sus conceptos internos)
	FindByID(ctx context.Context, id string) (*domain.Settlement, error)

	// FindByContractAndPeriod busca si ya existe una liquidación para ese mes específico.
	// Clave para evitar liquidaciones duplicadas del mismo contrato.
	FindByContractAndPeriod(ctx context.Context, contractID string, period string) (*domain.Settlement, error)
}
