package ports

import (
	"context"
	"inmo.platform/contexts/contracts/internal/domain"
)

type ContractRepository interface {
	// Save guarda un contrato en estado Draft o actualiza sus datos básicos
	Save(ctx context.Context, contract *domain.Contract) error

	// FindByID recupera un contrato específico por su identificador único
	FindByID(ctx context.Context, id string) (*domain.Contract, error)

	// FindAllActive recupera los contratos con estado ACTIVE (útil para el Worker de Ajustes)
	FindAllActive(ctx context.Context) ([]*domain.Contract, error)
}
