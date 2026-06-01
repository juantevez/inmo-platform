package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
)

type PostgresProfileRepository struct {
	db *sql.DB
}

func NewPostgresProfileRepository(db *sql.DB) *PostgresProfileRepository {
	return &PostgresProfileRepository{db: db}
}

// Save inserta o actualiza (Upsert) un perfil de negocio en inmo_catalog_db
func (r *PostgresProfileRepository) Save(ctx context.Context, profile *domain.Profile) error {
	query := `
		INSERT INTO profiles (
			user_id, first_name, last_name, dni_cuit, phone, 
			profile_type, company_name, license_number, status, 
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id) DO UPDATE SET
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			phone = EXCLUDED.phone,
			company_name = EXCLUDED.company_name,
			license_number = EXCLUDED.license_number,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP;`

	// Helpers para manejar campos opcionales que pueden ir como NULL a Postgres
	var compName, licNum sql.NullString
	if profile.CompanyName() != "" {
		compName = sql.NullString{String: profile.CompanyName(), Valid: true}
	}
	if profile.LicenseNumber() != "" {
		licNum = sql.NullString{String: profile.LicenseNumber(), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, query,
		profile.UserID(),
		profile.FirstName(),
		profile.LastName(),
		profile.DniCuit(),
		profile.Phone(),
		string(profile.ProfileType()),
		compName,
		licNum,
		string(profile.Status()),
		profile.CreatedAt(),
		profile.UpdatedAt(),
	)

	if err != nil {
		return fmt.Errorf("error al guardar el perfil de negocio: %w", err)
	}
	return nil
}

// FindByID busca un perfil por la clave primaria (user_id que viene del JWT)
func (r *PostgresProfileRepository) FindByID(ctx context.Context, userID string) (*domain.Profile, error) {
	query := `
		SELECT user_id, first_name, last_name, dni_cuit, phone, 
		       profile_type, company_name, license_number, status, created_at, updated_at 
		FROM profiles WHERE user_id = $1`

	row := r.db.QueryRowContext(ctx, query, userID)
	return r.scanRow(row)
}

// FindByDniCuit busca por el documento/CUIT para evitar duplicados en el negocio
func (r *PostgresProfileRepository) FindByDniCuit(ctx context.Context, dniCuit string) (*domain.Profile, error) {
	query := `
		SELECT user_id, first_name, last_name, dni_cuit, phone, 
		       profile_type, company_name, license_number, status, created_at, updated_at 
		FROM profiles WHERE dni_cuit = $1`

	row := r.db.QueryRowContext(ctx, query, dniCuit)
	return r.scanRow(row)
}

// Helper privado para centralizar el escaneo de filas y desmapeo de NullStrings
func (r *PostgresProfileRepository) scanRow(row *sql.Row) (*domain.Profile, error) {
	var uID, fName, lName, dni, phone, pType, status string
	var compName, licNum sql.NullString
	var createdAt, updatedAt time.Time

	err := row.Scan(&uID, &fName, &lName, &dni, &phone, &pType, &compName, &licNum, &status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al escanear fila de perfil: %w", err)
	}

	// Volvemos a strings vacíos si eran NULL en la base de datos
	cName := ""
	if compName.Valid {
		cName = compName.String
	}
	lNumber := ""
	if licNum.Valid {
		lNumber = licNum.String
	}

	return domain.ReconstructProfile(
		uID, fName, lName, dni, phone,
		domain.ProfileType(pType),
		cName, lNumber,
		domain.ProfileStatus(status),
		createdAt, updatedAt,
	), nil
}
