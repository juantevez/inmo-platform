package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/finances/internal/domain"
)

type PostgresSettlementRepository struct {
	db *sql.DB
}

// NewPostgresSettlementRepository instancias el adaptador de persistencia
func NewPostgresSettlementRepository(db *sql.DB) *PostgresSettlementRepository {
	return &PostgresSettlementRepository{db: db}
}

// Save inserta el agregado Settlement completo por primera vez de forma atómica
func (r *PostgresSettlementRepository) Save(ctx context.Context, s *domain.Settlement) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar la transacción de guardado: %w", err)
	}
	defer tx.Rollback() // Se ejecuta si hay un return antes del Commit

	// 1. Insertar la cabecera (Settlement)
	queryHeader := `
		INSERT INTO settlements (id, contract_id, period, status, created_at, closed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = tx.ExecContext(ctx, queryHeader, s.ID(), s.ContractID(), s.Period(), string(s.Status()), s.CreatedAt(), s.ClosedAt())
	if err != nil {
		return fmt.Errorf("error al insertar cabecera de liquidación: %w", err)
	}

	// 2. Insertar los conceptos hijos si los tuviera (fábrica los inicia vacíos por defecto, pero robustez ante todo)
	if len(s.Concepts()) > 0 {
		if err := r.insertConcepts(ctx, tx, s.ID(), s.Concepts()); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error al confirmar (commit) la transacción de guardado: %w", err)
	}

	return nil
}

// Update sincroniza el estado mutado de la liquidación y refresca la lista de conceptos hijos
func (r *PostgresSettlementRepository) Update(ctx context.Context, s *domain.Settlement) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar la transacción de actualización: %w", err)
	}
	defer tx.Rollback()

	// 1. Actualizar datos de la cabecera (Estado y fecha de cierre)
	queryHeader := `
		UPDATE settlements 
		SET status = $1, closed_at = $2 
		WHERE id = $3
	`
	_, err = tx.ExecContext(ctx, queryHeader, string(s.Status()), s.ClosedAt(), s.ID())
	if err != nil {
		return fmt.Errorf("error al actualizar cabecera de liquidación: %w", err)
	}

	// 2. Manejo del detalle (Estrategia rápida para desarrollo local/MVP):
	// Borramos los conceptos previos asignados a esta liquidación y volvemos a insertar el estado actual de la lista en memoria.
	// Esto asegura consistencia total si se borraron o agregaron elementos.
	queryDeleteConcepts := `DELETE FROM settlement_concepts WHERE settlement_id = $1`
	_, err = tx.ExecContext(ctx, queryDeleteConcepts, s.ID())
	if err != nil {
		return fmt.Errorf("error al limpiar conceptos viejos para actualizar: %w", err)
	}

	if len(s.Concepts()) > 0 {
		if err := r.insertConcepts(ctx, tx, s.ID(), s.Concepts()); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error al confirmar la transacción de actualización: %w", err)
	}

	return nil
}

// FindByID recupera el registro hidratando por completo el dominio con sus sub-entidades
func (r *PostgresSettlementRepository) FindByID(ctx context.Context, id string) (*domain.Settlement, error) {
	// 1. Buscar cabecera
	queryHeader := `SELECT id, contract_id, period, status, created_at, closed_at FROM settlements WHERE id = $1`
	row := r.db.QueryRowContext(ctx, queryHeader, id)

	var sID, contractID, period, statusStr string
	var createdAt time.Time
	var closedAt *time.Time

	err := row.Scan(&sID, &contractID, &period, &statusStr, &createdAt, &closedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No existe, retornamos nil sin error según contrato del puerto
		}
		return nil, fmt.Errorf("error al escanear cabecera de liquidación: %w", err)
	}

	// 2. Buscar conceptos vinculados
	concepts, err := r.loadConcepts(ctx, id)
	if err != nil {
		return nil, err
	}

	// 3. Reconstitución del dominio sin disparar reglas de la fábrica
	settlement := domain.ReconstituteSettlement(
		sID,
		contractID,
		period,
		domain.SettlementStatus(statusStr),
		concepts,
		createdAt,
		closedAt,
	)

	return settlement, nil
}

// FindByContractAndPeriod busca coincidencia exacta para el control de duplicados de la aplicación
func (r *PostgresSettlementRepository) FindByContractAndPeriod(ctx context.Context, contractID string, period string) (*domain.Settlement, error) {
	query := `SELECT id FROM settlements WHERE contract_id = $1 AND period = $2`
	var id string
	err := r.db.QueryRowContext(ctx, query, contractID, period).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("error al buscar liquidación por contrato y período: %w", err)
	}

	// Si encontramos el ID, delegamos a FindByID para recuperar el objeto completamente armado
	return r.FindByID(ctx, id)
}

// =========================================================================
// Métodos Auxiliares de Infraestructura (No expuestos en la interfaz pública)
// =========================================================================

func (r *PostgresSettlementRepository) insertConcepts(ctx context.Context, tx *sql.Tx, settlementID string, concepts []*domain.SettlementConcept) error {
	queryConcept := `
		INSERT INTO settlement_concepts (id, settlement_id, description, concept_type, amount)
		VALUES ($1, $2, $3, $4, $5)
	`
	for _, c := range concepts {
		_, err := tx.ExecContext(ctx, queryConcept, c.ID(), settlementID, c.Description(), string(c.Type()), c.Amount())
		if err != nil {
			return fmt.Errorf("error al insertar concepto [%s]: %w", c.Description(), err)
		}
	}
	return nil
}

func (r *PostgresSettlementRepository) loadConcepts(ctx context.Context, settlementID string) ([]*domain.SettlementConcept, error) {
	query := `SELECT id, description, concept_type, amount FROM settlement_concepts WHERE settlement_id = $1`
	rows, err := r.db.QueryContext(ctx, query, settlementID)
	if err != nil {
		return nil, fmt.Errorf("error al consultar conceptos de liquidación: %w", err)
	}
	defer rows.Close()

	var concepts []*domain.SettlementConcept
	for rows.Next() {
		var cID, desc, typeStr string
		var amount float64
		if err := rows.Scan(&cID, &desc, &typeStr, &amount); err != nil {
			return nil, fmt.Errorf("error al escanear concepto de liquidación: %w", err)
		}

		concept := domain.ReconstituteConcept(cID, desc, domain.ConceptType(typeStr), amount)
		concepts = append(concepts, concept)
	}

	return concepts, nil
}
