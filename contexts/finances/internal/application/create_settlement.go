package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/finances/internal/domain"
	"inmo.platform/contexts/finances/internal/ports"
)

var (
	ErrContractNotFoundOrInactive = errors.New("el contrato no existe o no se encuentra activo para liquidación")
	ErrSettlementAlreadyExists    = errors.New("ya existe una liquidación generada para este contrato y período")
)

// CreateSettlementCommand define los datos de entrada requeridos para abrir la liquidación
type CreateSettlementCommand struct {
	ID         string `json:"id"`          // ID generado por el cliente/adaptador (UUIDv4)
	ContractID string `json:"contract_id"` // Referencia externa al contrato
	Period     string `json:"period"`      // Formato "YYYY-MM"
}

// CreateSettlementUseCase orquesta la lógica de creación
type CreateSettlementUseCase struct {
	repo            ports.SettlementRepository
	contractService ports.ContractService
}

// NewCreateSettlementUseCase es el constructor del Caso de Uso (Inyección de dependencias)
func NewCreateSettlementUseCase(repo ports.SettlementRepository, contractService ports.ContractService) *CreateSettlementUseCase {
	return &CreateSettlementUseCase{
		repo:            repo,
		contractService: contractService,
	}
}

// Execute ejecuta la lógica del comando respetando las fronteras arquitectónicas
func (uc *CreateSettlementUseCase) Execute(ctx context.Context, cmd CreateSettlementCommand) error {
	// 1. Validar reglas de aplicación básicas sobre los datos de entrada
	if cmd.ID == "" || cmd.ContractID == "" || cmd.Period == "" {
		return errors.New("los campos id, contract_id y period son obligatorios")
	}

	// 2. Colaboración con puerto externo: Verificar estado del contrato
	isActive, err := uc.contractService.IsContractActive(ctx, cmd.ContractID)
	if err != nil {
		return fmt.Errorf("error al verificar el estado del contrato: %w", err)
	}
	if !isActive {
		return ErrContractNotFoundOrInactive
	}

	// 3. Colaboración con repositorio: Prevenir duplicación del período para este contrato
	existing, err := uc.repo.FindByContractAndPeriod(ctx, cmd.ContractID, cmd.Period)
	if err != nil {
		return fmt.Errorf("error al comprobar duplicados de liquidación: %w", err)
	}
	if existing != nil {
		return ErrSettlementAlreadyExists
	}

	// 4. Invocar la Fábrica del Dominio para construir la raíz del Agregado
	settlement, err := domain.NewSettlement(cmd.ID, cmd.ContractID, cmd.Period)
	if err != nil {
		return fmt.Errorf("error de dominio al instanciar liquidación: %w", err)
	}

	// 5. Guardar el agregado completo en la persistencia de forma atómica
	if err := uc.repo.Save(ctx, settlement); err != nil {
		return fmt.Errorf("error al persistir la nueva liquidación: %w", err)
	}

	return nil
}
