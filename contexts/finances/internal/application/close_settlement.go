package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/finances/internal/ports"
)

type CloseSettlementUseCase struct {
	repo       ports.SettlementRepository
	dispatcher ports.EventDispatcher
}

func NewCloseSettlementUseCase(repo ports.SettlementRepository, dispatcher ports.EventDispatcher) *CloseSettlementUseCase {
	return &CloseSettlementUseCase{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

func (uc *CloseSettlementUseCase) Execute(ctx context.Context, settlementID string) error {
	if settlementID == "" {
		return errors.New("el ID de la liquidación es obligatorio para proceder al cierre")
	}

	// 1. Recuperar el Agregado
	settlement, err := uc.repo.FindByID(ctx, settlementID)
	if err != nil {
		return fmt.Errorf("error al buscar la liquidación para cierre: %w", err)
	}
	if settlement == nil {
		return ErrSettlementNotFound
	}

	// 2. Ejecutar la regla de negocio del dominio (Cerrar y estampar timestamp de bloqueo)
	if err := settlement.Close(); err != nil {
		return fmt.Errorf("error de negocio al intentar cerrar la liquidación: %w", err)
	}

	// 3. Persistir el cambio de estado atómicamente
	if err := uc.repo.Update(ctx, settlement); err != nil {
		return fmt.Errorf("error al guardar el estado de cierre de la liquidación: %w", err)
	}

	// 4. Despachar Evento de Dominio al exterior del Bounded Context
	// Notificamos que se cerró la liquidación para que otros subsistemas emitan los comprobantes o mails
	if err := uc.dispatcher.PublishSettlementClosed(ctx, settlement.ID()); err != nil {
		// Loggeamos el error pero no rompemos la transacción del negocio,
		// o sí, dependiendo de tu política de consistencia eventual (en este caso seguimos)
		fmt.Printf("[⚠️ Event Error] No se pudo despachar el evento de cierre para %s: %v\n", settlement.ID(), err)
	}

	return nil
}
