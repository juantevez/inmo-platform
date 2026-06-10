package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"inmo.platform/contexts/maintenance/internal/domain"
)

type PostgresProviderRepository struct {
	db *sql.DB
}

func NewPostgresProviderRepository(db *sql.DB) *PostgresProviderRepository {
	return &PostgresProviderRepository{db: db}
}

// Save inserta un nuevo proveedor técnico en la tabla proveedores_tecnicos
func (r *PostgresProviderRepository) Save(ctx context.Context, p *domain.Provider) error {
	query := `
		INSERT INTO proveedores_tecnicos (
			id, user_id, razon_social, cuit_cuil, rubro,
			cbu_pago, alias_pago, disponible_urgencias,
			status, registered_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := r.db.ExecContext(ctx, query,
		p.ID(),
		p.UserID(),
		p.RazonSocial(),
		p.CuitCuil(),
		string(p.Rubro()),
		nullableString(p.CbuPago()),
		nullableString(p.AliasPago()),
		p.DisponibleUrgencias(),
		string(p.Status()),
		nullableString(p.RegisteredBy()),
		p.CreatedAt(),
		p.UpdatedAt(),
	)
	return err
}

// Update persiste cambios sobre un proveedor existente
func (r *PostgresProviderRepository) Update(ctx context.Context, p *domain.Provider) error {
	query := `
		UPDATE proveedores_tecnicos SET
			razon_social         = $1,
			cbu_pago             = $2,
			alias_pago           = $3,
			disponible_urgencias = $4,
			status               = $5,
			updated_at           = $6
		WHERE id = $7
	`

	_, err := r.db.ExecContext(ctx, query,
		p.RazonSocial(),
		nullableString(p.CbuPago()),
		nullableString(p.AliasPago()),
		p.DisponibleUrgencias(),
		string(p.Status()),
		p.UpdatedAt(),
		p.ID(),
	)
	return err
}

// FindByID recupera un proveedor por su ID de agregado
func (r *PostgresProviderRepository) FindByID(ctx context.Context, id string) (*domain.Provider, error) {
	query := `
		SELECT id, user_id, razon_social, cuit_cuil, rubro,
		       cbu_pago, alias_pago, disponible_urgencias,
		       status, registered_by, created_at, updated_at
		FROM proveedores_tecnicos
		WHERE id = $1
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanProvider(row)
}

// FindByUserID recupera el proveedor asociado a un user_id de auth_db
func (r *PostgresProviderRepository) FindByUserID(ctx context.Context, userID string) (*domain.Provider, error) {
	query := `
		SELECT id, user_id, razon_social, cuit_cuil, rubro,
		       cbu_pago, alias_pago, disponible_urgencias,
		       status, registered_by, created_at, updated_at
		FROM proveedores_tecnicos
		WHERE user_id = $1
	`
	row := r.db.QueryRowContext(ctx, query, userID)
	return r.scanProvider(row)
}

// FindByCuitCuil busca un proveedor por CUIT/CUIL para validar unicidad
func (r *PostgresProviderRepository) FindByCuitCuil(ctx context.Context, cuitCuil string) (*domain.Provider, error) {
	query := `
		SELECT id, user_id, razon_social, cuit_cuil, rubro,
		       cbu_pago, alias_pago, disponible_urgencias,
		       status, registered_by, created_at, updated_at
		FROM proveedores_tecnicos
		WHERE cuit_cuil = $1
	`
	row := r.db.QueryRowContext(ctx, query, cuitCuil)
	return r.scanProvider(row)
}

// FindByRubro lista proveedores ACTIVE de un rubro específico
func (r *PostgresProviderRepository) FindByRubro(ctx context.Context, rubro domain.RubroTecnico) ([]*domain.Provider, error) {
	query := `
		SELECT id, user_id, razon_social, cuit_cuil, rubro,
		       cbu_pago, alias_pago, disponible_urgencias,
		       status, registered_by, created_at, updated_at
		FROM proveedores_tecnicos
		WHERE rubro = $1 AND status = 'ACTIVE'
		ORDER BY razon_social ASC
	`
	return r.queryProviders(ctx, query, string(rubro))
}

// FindAvailableForEmergency lista proveedores ACTIVE con disponibilidad de urgencias para un rubro
func (r *PostgresProviderRepository) FindAvailableForEmergency(ctx context.Context, rubro domain.RubroTecnico) ([]*domain.Provider, error) {
	query := `
		SELECT id, user_id, razon_social, cuit_cuil, rubro,
		       cbu_pago, alias_pago, disponible_urgencias,
		       status, registered_by, created_at, updated_at
		FROM proveedores_tecnicos
		WHERE rubro = $1
		  AND status = 'ACTIVE'
		  AND disponible_urgencias = TRUE
		ORDER BY razon_social ASC
	`
	return r.queryProviders(ctx, query, string(rubro))
}

// --- Helpers privados ---

// scanProvider desmapea una fila de Postgres al agregado Provider
func (r *PostgresProviderRepository) scanProvider(row *sql.Row) (*domain.Provider, error) {
	var (
		id, userID, razonSocial, cuitCuil, rubroStr, statusStr string
		cbuPago, aliasPago, registeredBy                       sql.NullString
		disponibleUrgencias                                    bool
		createdAt, updatedAt                                   time.Time
	)

	err := row.Scan(
		&id, &userID, &razonSocial, &cuitCuil, &rubroStr,
		&cbuPago, &aliasPago, &disponibleUrgencias,
		&statusStr, &registeredBy, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // no encontrado — no es un error
	}
	if err != nil {
		return nil, err
	}

	return domain.ReconstructProvider(
		id,
		userID,
		razonSocial,
		cuitCuil,
		domain.RubroTecnico(rubroStr),
		cbuPago.String,
		aliasPago.String,
		disponibleUrgencias,
		domain.ProviderStatus(statusStr),
		registeredBy.String,
		createdAt,
		updatedAt,
	), nil
}

// queryProviders ejecuta una query que devuelve múltiples proveedores
func (r *PostgresProviderRepository) queryProviders(ctx context.Context, query string, args ...interface{}) ([]*domain.Provider, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []*domain.Provider
	for rows.Next() {
		var (
			id, userID, razonSocial, cuitCuil, rubroStr, statusStr string
			cbuPago, aliasPago, registeredBy                       sql.NullString
			disponibleUrgencias                                    bool
			createdAt, updatedAt                                   time.Time
		)

		if err := rows.Scan(
			&id, &userID, &razonSocial, &cuitCuil, &rubroStr,
			&cbuPago, &aliasPago, &disponibleUrgencias,
			&statusStr, &registeredBy, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		providers = append(providers, domain.ReconstructProvider(
			id,
			userID,
			razonSocial,
			cuitCuil,
			domain.RubroTecnico(rubroStr),
			cbuPago.String,
			aliasPago.String,
			disponibleUrgencias,
			domain.ProviderStatus(statusStr),
			registeredBy.String,
			createdAt,
			updatedAt,
		))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return providers, nil
}

// nullableString convierte un string vacío en sql.NullString{Valid: false}
// para evitar guardar strings vacíos donde debería ir NULL en Postgres
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
