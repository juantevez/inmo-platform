package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// OutboxRepository define lo que la aplicación necesita para persistir eventos de forma atómica
type OutboxRepository interface {
	SaveTx(ctx context.Context, tx *sql.Tx, eventName string, payload []byte) error
}

// SettlementRepositoryTx necesita soportar transacciones para que el caso de uso controle el Commit/Rollback
type SettlementRepositoryTx interface {
	// Asumo que ya tenés un método para buscar y actualizar, pero necesitamos extraer la Tx
	BeginTx(ctx context.Context) (*sql.Tx, error)
	UpdateStatusTx(ctx context.Context, tx *sql.Tx, settlementId string, status string) error
}

type CloseSettlementUseCase struct {
	repo       SettlementRepositoryTx
	outboxRepo OutboxRepository
}

// Jubilamos el viejo eventDispatcherStub e inyectamos OutboxRepository
func NewCloseSettlementUseCase(repo SettlementRepositoryTx, outboxRepo OutboxRepository) *CloseSettlementUseCase {
	return &CloseSettlementUseCase{
		repo:       repo,
		outboxRepo: outboxRepo,
	}
}

type SettlementClosedEventPayload struct {
	SettlementID string    `json:"settlement_id"`
	ContractID   string    `json:"contract_id"`
	Period       string    `json:"period"`
	ClosedAt     time.Time `json:"closed_at"`
}

func (uc *CloseSettlementUseCase) Execute(ctx context.Context, settlementId string) error {
	// 1. Iniciar la transacción de base de datos
	tx, err := uc.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("error al iniciar transaccion: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Si algo falla o no se confirma con Commit, se deshace todo

	// 2. Ejecutar la lógica de negocio: Cambiar el estado de la liquidación a CLOSED
	// (Acá podrías tener una búsqueda previa si tu modelo de dominio lo requiere)
	err = uc.repo.UpdateStatusTx(ctx, tx, settlementId, "CLOSED")
	if err != nil {
		return fmt.Errorf("error al actualizar estado de la liquidacion: %w", err)
	}

	// 3. Construir el Payload del evento de dominio
	eventPayload := SettlementClosedEventPayload{
		SettlementID: settlementId,
		ClosedAt:     time.Now(),
		// Nota: Si necesitás contract_id o period, deberías haber hecho un Select previo con el 'tx'
	}

	jsonPayload, err := json.Marshal(eventPayload)
	if err != nil {
		return fmt.Errorf("error al serializar evento: %w", err)
	}

	// 4. Persistir el evento en la bandeja de salida (Outbox) DENTRO de la misma transacción
	err = uc.outboxRepo.SaveTx(ctx, tx, "finance.settlement.closed", jsonPayload)
	if err != nil {
		return fmt.Errorf("error al guardar evento en outbox: %w", err)
	}

	// 5. Si todo salió bien, consolidamos la transacción en la BD
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error al hacer commit de la transaccion: %w", err)
	}

	return nil
}
