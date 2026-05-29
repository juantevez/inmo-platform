package application

import (
	"context"
	"database/sql"
	"fmt"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/contexts/contracts/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type ActivateContractUseCase struct {
	dbPool       *sql.DB
	contractRepo ports.ContractRepository
}

func NewActivateContractUseCase(dbPool *sql.DB, repo ports.ContractRepository) *ActivateContractUseCase {
	return &ActivateContractUseCase{
		dbPool:       dbPool,
		contractRepo: repo,
	}
}

func (uc *ActivateContractUseCase) Execute(ctx context.Context, contractID string) error {
	// 1. Iniciar la Transacción SQL para asegurar atomicidad (Contrato + Outbox)
	tx, err := uc.dbPool.BeginTx(ctx, nil)
	if err != nil {
		return apperr.NewInternal("no se pudo iniciar la transacción de activación", err)
	}
	defer tx.Rollback()

	// 2. Buscar el contrato actual
	contract, err := uc.contractRepo.FindByID(ctx, contractID)
	if err != nil {
		return fmt.Errorf("error al buscar el contrato: %w", err)
	}
	if contract == nil {
		return apperr.NewNotFound(fmt.Sprintf("contrato con ID %s no encontrado", contractID), nil)
	}

	// 3. Ejecutar la regla de negocio en el Agregado de Dominio (Muta estado y genera evento)
	if err := contract.Activate(); err != nil {
		return apperr.NewPreconditionFailed(err.Error(), err)
	}

	// 4. Hacer el Type Assertion para extraer el repositorio real que sabe manejar transacciones de Postgres
	type TxContractRepository interface {
		SaveWithTx(ctx context.Context, tx *sql.Tx, c *domain.Contract) error
	}

	txRepo, ok := uc.contractRepo.(TxContractRepository)
	if !ok {
		return apperr.NewInternal("el repositorio configurado no soporta transacciones outbox", nil)
	}

	// 5. Guardar el estado del contrato y sus eventos de outbox dentro de la Tx
	if err := txRepo.SaveWithTx(ctx, tx, contract); err != nil {
		return err
	}

	// 6. Hacer el Commit atómico en la base de datos
	if err := tx.Commit(); err != nil {
		return apperr.NewInternal("error al confirmar la transacción de activación", err)
	}

	return nil
}
