package application

import (
	// 1. Cosas INTERNAS del propio módulo de contratos (antepone su propio nombre de módulo)
	"context"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/contexts/contracts/internal/ports"
	"time"

	// 2. Cosas EXTERNAS que vienen del Shared Kernel (usan el prefijo del workspace)
	"inmo.platform/shared/pkg/apperr"
)

type CreateContractDTO struct {
	ID               string
	PropertyID       string
	TenantID         string
	OwnerID          string
	Amount           float64
	Currency         string
	StartDate        time.Time
	EndDate          time.Time
	AdjustmentIndex  string
	AdjustmentPeriod int
}

type CreateContractUseCase struct {
	contractRepo ports.ContractRepository
}

func NewCreateContractUseCase(repo ports.ContractRepository) *CreateContractUseCase {
	return &CreateContractUseCase{contractRepo: repo}
}

func (uc *CreateContractUseCase) Execute(ctx context.Context, dto CreateContractDTO) error {
	// 1. Validar e instanciar Value Objects
	rent, err := domain.NewRentAmount(dto.Amount, dto.Currency)
	if err != nil {
		return apperr.NewBadRequest(err.Error(), err)
	}

	timeline, err := domain.NewTimeline(dto.StartDate, dto.EndDate)
	if err != nil {
		return apperr.NewBadRequest(err.Error(), err)
	}

	period, err := domain.NewAdjustmentPeriod(dto.AdjustmentPeriod)
	if err != nil {
		return apperr.NewBadRequest(err.Error(), err)
	}

	// 2. Instanciar el Agregado de Dominio (Nace en DRAFT)
	contract := domain.NewContract(
		dto.ID,
		dto.PropertyID,
		dto.TenantID,
		dto.OwnerID,
		rent,
		timeline,
		domain.AdjustmentIndex(dto.AdjustmentIndex),
		period,
	)

	// 3. Persistir en base de datos
	if err := uc.contractRepo.Save(ctx, contract); err != nil {
		return err
	}

	return nil
}
