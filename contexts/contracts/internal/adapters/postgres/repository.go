package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type ContractRepository struct {
	db *sql.DB
}

func NewContractRepository(db *sql.DB) *ContractRepository {
	return &ContractRepository{db: db}
}

// Save implementa el puerto básico (fuera de una transacción manual externa)
func (r *ContractRepository) Save(ctx context.Context, c *domain.Contract) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.NewInternal("no se pudo iniciar tx para guardar contrato", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.SaveWithTx(ctx, tx, c); err != nil {
		return err
	}

	return tx.Commit()
}

// SaveWithTx guarda el agregado Contract y sus eventos en las tablas correspondientes de forma atómica
func (r *ContractRepository) SaveWithTx(ctx context.Context, tx *sql.Tx, c *domain.Contract) error {
	// 1. Query para guardar o actualizar el Contrato
	contractQuery := `
		INSERT INTO contracts (
			id, property_id, tenant_id, owner_id, rent_amount, currency, 
			start_date, end_date, adjustment_index, adjustment_period_months, state, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			rent_amount = EXCLUDED.rent_amount,
			updated_at = CURRENT_TIMESTAMP;
	`

	_, err := tx.ExecContext(ctx, contractQuery,
		c.ID(), c.PropertyID(), c.TenantID(), c.OwnerID(),
		c.RentAmount().Amount(), c.RentAmount().Currency(),
		c.Timeline().StartDate(), c.Timeline().EndDate(),
		string(c.Index()), int(c.AdjustmentPeriod()), string(c.State()),
		c.CreatedAt(), // Asumiendo que agregás un getter c.CreatedAt() o mapeás time.Now()
	)
	if err != nil {
		return apperr.NewInternal("error al guardar el contrato en postgres con tx", err)
	}

	// 2. Guardar los eventos de dominio acumulados en la tabla contracts_outbox_events
	for _, event := range c.PullEvents() {
		payloadBytes, err := json.Marshal(event)
		if err != nil {
			return apperr.NewInternal("error al serializar evento de contrato para outbox", err)
		}

		outboxQuery := `
			INSERT INTO contracts_outbox_events (id, aggregate_type, aggregate_id, event_name, payload, status)
			VALUES ($1, $2, $3, $4, $5, 'PENDING');
		`
		eventID := fmt.Sprintf("evt-ctr-%s-%d", event.AggregateID(), time.Now().UnixNano())

		_, err = tx.ExecContext(ctx, outboxQuery,
			eventID,
			"Contract",
			event.AggregateID(),
			event.EventName(),
			payloadBytes,
		)
		if err != nil {
			return apperr.NewInternal("error al insertar en la tabla outbox de contratos", err)
		}
	}

	return nil
}

// FindByID recupera un contrato reconstruyendo sus Value Objects desde la base de datos
func (r *ContractRepository) FindByID(ctx context.Context, id string) (*domain.Contract, error) {
	query := `
		SELECT property_id, tenant_id, owner_id, rent_amount, currency, 
		       start_date, end_date, adjustment_index, adjustment_period_months, state, created_at
		FROM contracts 
		WHERE id = $1;
	`

	row := r.db.QueryRowContext(ctx, query, id)

	var (
		propID, tenantID, ownerID, currency, stateStr, idxStr string
		amount                                                float64
		start, end, createdAt                                 time.Time
		periodMonths                                          int
	)

	err := row.Scan(&propID, &tenantID, &ownerID, &amount, &currency, &start, &end, &idxStr, &periodMonths, &stateStr, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Not found controlado por la aplicación
		}
		return nil, apperr.NewInternal("error al escanear fila de contrato", err)
	}

	// Reconstrucción segura de Value Objects aplicando las reglas de fábrica (mapeo inverso)
	rent, err := domain.NewRentAmount(amount, currency)
	if err != nil {
		return nil, apperr.NewInternal("datos corrompidos de monto en bd", err)
	}

	timeline, err := domain.NewTimeline(start, end)
	if err != nil {
		return nil, apperr.NewInternal("datos corrompidos de fechas en bd", err)
	}

	// Usamos un truco limpio de Go para saltarnos la validación de negocio al reconstruir desde la BD,
	// o usamos los constructores públicos. Reconstruimos el objeto hidratado:
	contract := domain.RehydrateContract(
		id, propID, tenantID, ownerID, rent, timeline,
		domain.AdjustmentIndex(idxStr), domain.AdjustmentPeriod(periodMonths),
		domain.ContractState(stateStr), createdAt,
	)

	return contract, nil
}

// FindAllActive recupera contratos activos (necesario para el puerto/interfaz completo)
func (r *ContractRepository) FindAllActive(ctx context.Context) ([]*domain.Contract, error) {
	// Dejamos el esqueleto por si luego querés meter el Worker de Ajustes por Índice
	return nil, nil
}
