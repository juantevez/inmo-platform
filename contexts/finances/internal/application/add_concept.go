package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/finances/internal/domain"
	"inmo.platform/contexts/finances/internal/ports"
)

var ErrSettlementNotFound = errors.New("la liquidación especificada no existe")

// AddConceptCommand transporta los datos de entrada para inyectar el concepto
type AddConceptCommand struct {
	SettlementID string             `json:"settlement_id"`
	ConceptID    string             `json:"concept_id"` // UUIDv4 generado externamente
	Description  string             `json:"description"`
	ConceptType  domain.ConceptType `json:"concept_type"`
	Amount       float64            `json:"amount"`
}

type AddConceptUseCase struct {
	repo ports.SettlementRepository
}

func NewAddConceptUseCase(repo ports.SettlementRepository) *AddConceptUseCase {
	return &AddConceptUseCase{repo: repo}
}

func (uc *AddConceptUseCase) Execute(ctx context.Context, cmd AddConceptCommand) error {
	// 1. Validaciones básicas de la estructura del comando
	if cmd.SettlementID == "" || cmd.ConceptID == "" || cmd.Amount <= 0 {
		return errors.New("los campos settlement_id, concept_id y un monto mayor a cero son obligatorios")
	}

	// 2. Recuperar el Agregado Completo a través del puerto
	settlement, err := uc.repo.FindByID(ctx, cmd.SettlementID)
	if err != nil {
		return fmt.Errorf("error al buscar la liquidación: %w", err)
	}
	if settlement == nil {
		return ErrSettlementNotFound
	}

	// 3. Delegar la lógica y el control de invariantes a la raíz del Agregado
	// El dominio se va a quejar acá si el estado no es "OPEN"
	err = settlement.AddConcept(cmd.ConceptID, cmd.Description, cmd.ConceptType, cmd.Amount)
	if err != nil {
		return fmt.Errorf("error de negocio al agregar concepto: %w", err)
	}

	// 4. Guardar los cambios (sincronizar el estado mutado)
	if err := uc.repo.Update(ctx, settlement); err != nil {
		return fmt.Errorf("error al actualizar el agregado en base de datos: %w", err)
	}

	return nil
}
