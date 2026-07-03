package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/auth-identity/internal/domain"
)

type PostgresUserRepository struct {
	db *DB
}

func NewPostgresUserRepository(db *DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// Save inserta un usuario nuevo, su proveedor y sus roles iniciales en una transacción atómica (UC-01/04/05)
func (r *PostgresUserRepository) Save(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
	tx, err := r.db.Pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error al iniciar transaccion de guardado: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Si algo falla antes del Commit, limpia todo automáticamente

	// 1. Insertar el usuario núcleo
	userQuery := `INSERT INTO users (id, email, status, phone, phone_verified_at, created_at) 
                  VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = tx.ExecContext(ctx, userQuery,
		user.ID(),
		user.Email(),
		string(user.Status()),
		user.Phone(),
		user.PhoneVerifiedAt(),
		user.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("falló inserción de usuario núcleo: %w", err)
	}

	// 2. Manejar el password hash según el proveedor
	var pwdHash sql.NullString
	if provider.Name() == domain.ProviderEmail {
		pwdHash = sql.NullString{String: provider.PasswordHash(), Valid: true}
	} else {
		pwdHash = sql.NullString{Valid: false}
	}

	// 3. Insertar el proveedor inicial
	provQuery := `INSERT INTO identity_providers (id, user_id, provider_name, provider_user_id, password_hash) 
                  VALUES ($1, $2, $3, $4, $5)`

	_, err = tx.ExecContext(ctx, provQuery,
		provider.ID(),
		user.ID(),
		string(provider.Name()),
		provider.ProviderUserID(),
		pwdHash,
	)
	if err != nil {
		return fmt.Errorf("falló inserción de identity provider inicial: %w", err)
	}

	// 🚀 4. NUEVO: Insertar los roles iniciales del usuario asociados en la tabla asociativa
	if len(roles) == 0 {
		// Estrategia preventiva: Si por alguna razón no viene ningún rol, le asignamos uno por defecto
		roles = []string{"INQUILINO"}
	}

	roleQuery := `INSERT INTO user_roles (user_id, role, created_at) VALUES ($1, $2, CURRENT_TIMESTAMP)`
	for _, role := range roles {
		_, err = tx.ExecContext(ctx, roleQuery, user.ID(), role)
		if err != nil {
			return fmt.Errorf("falló inserción del rol '%s' para el usuario: %w", role, err)
		}
	}

	return tx.Commit()
}

// Update actualiza el estado general del usuario (ej: pasar a ACTIVE, teléfono, etc) (UC-02/07)
func (r *PostgresUserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `UPDATE users SET status = $1, phone = $2, phone_verified_at = $3 WHERE id = $4`

	_, err := r.db.Pool.ExecContext(ctx, query,
		string(user.Status()),
		user.Phone(),
		user.PhoneVerifiedAt(),
		user.ID(),
	)
	if err != nil {
		return fmt.Errorf("error al actualizar usuario: %w", err)
	}
	return nil
}

// FindByID busca un usuario por su clave primaria
func (r *PostgresUserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE id = $1`
	row := r.db.Pool.QueryRowContext(ctx, query, id)

	var uID, email, status, phone string
	var phoneVerifiedAt sql.NullTime
	var createdAt time.Time

	err := row.Scan(&uID, &email, &status, &phone, &phoneVerifiedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al escanear usuario por ID: %w", err)
	}

	return r.reconstructUser(uID, email, status, phone, phoneVerifiedAt, createdAt), nil
}

// FindByEmail busca un usuario núcleo por su correo electrónico (UC-01/04/05)
func (r *PostgresUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE email = $1`
	row := r.db.Pool.QueryRowContext(ctx, query, email)

	var id, uEmail, status, phone string
	var phoneVerifiedAt sql.NullTime
	var createdAt time.Time

	err := row.Scan(&id, &uEmail, &status, &phone, &phoneVerifiedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al escanear usuario por email: %w", err)
	}

	return r.reconstructUser(id, uEmail, status, phone, phoneVerifiedAt, createdAt), nil
}

// Dentro de internal/adapters/postgres/user_repository.go
func (r *PostgresUserRepository) FindRolesByUserID(ctx context.Context, userID string) ([]string, error) {
	query := `SELECT role FROM user_roles WHERE user_id = $1`
	rows, err := r.db.Pool.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("error al consultar roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// FindProvider busca un método de login específico asociado a un usuario (UC-03)
func (r *PostgresUserRepository) FindProvider(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
	// ◄ CORREGIDO: Añadimos password_hash en la consulta por si se necesita en el Login posterior
	query := `SELECT id, provider_user_id, password_hash FROM identity_providers WHERE user_id = $1 AND provider_name = $2`
	row := r.db.Pool.QueryRowContext(ctx, query, userID, string(pType))

	var id, providerUserID string
	var passwordHash sql.NullString

	err := row.Scan(&id, &providerUserID, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al escanear proveedor de identidad: %w", err)
	}

	hash := ""
	if passwordHash.Valid {
		hash = passwordHash.String
	}

	return domain.ReconstructProvider(id, userID, pType, providerUserID, hash), nil
}

// FindByProviderKey busca si ya existe un ID externo registrado (ej: el 'sub' de Google) (UC-04/05)
func (r *PostgresUserRepository) FindByProviderKey(ctx context.Context, pType domain.ProviderType, providerUserID string) (*domain.User, error) {
	query := `
        SELECT u.id, u.email, u.status, u.phone, u.phone_verified_at, u.created_at 
        FROM users u
        INNER JOIN identity_providers ip ON u.id = ip.user_id
        WHERE ip.provider_name = $1 AND ip.provider_user_id = $2
    `
	row := r.db.Pool.QueryRowContext(ctx, query, string(pType), providerUserID)

	var id, email, status, phone string
	var phoneVerifiedAt sql.NullTime
	var createdAt time.Time

	err := row.Scan(&id, &email, &status, &phone, &phoneVerifiedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al buscar usuario por clave de proveedor: %w", err)
	}

	return r.reconstructUser(id, email, status, phone, phoneVerifiedAt, createdAt), nil
}

// AddProvider vincula un nuevo proveedor de identidad a un usuario existente (UC-06)
func (r *PostgresUserRepository) AddProvider(ctx context.Context, userID string, provider *domain.IdentityProvider) error {
	query := `INSERT INTO identity_providers (id, user_id, provider_name, provider_user_id, password_hash) 
              VALUES ($1, $2, $3, $4, $5)`

	// ◄ CORREGIDO: Mapeamos con el dinámico provider.Name() adaptado
	_, err := r.db.Pool.ExecContext(ctx, query,
		provider.ID(),
		userID,
		string(provider.Name()),
		provider.ProviderUserID(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("error al vincular nuevo proveedor de identidad: %w", err)
	}
	return nil
}

// SaveVerificationToken guarda el token UUID o OTP para posterior verificación (UC-01/07)
func (r *PostgresUserRepository) SaveVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	query := `INSERT INTO verification_tokens (token, token_type, user_id, expires_at, used_at) 
              VALUES ($1, $2, $3, $4, $5)`

	_, err := r.db.Pool.ExecContext(ctx, query,
		token.Value(),
		string(token.Type()),
		token.UserID(),
		token.ExpiresAt(),
		token.UsedAt(),
	)
	if err != nil {
		return fmt.Errorf("error al guardar token de verificacion: %w", err)
	}
	return nil
}

// FindVerificationToken busca un token para validar su existencia y estado (UC-02/07)
func (r *PostgresUserRepository) FindVerificationToken(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
	query := `SELECT token, token_type, user_id, expires_at, used_at FROM verification_tokens WHERE token = $1 AND token_type = $2`
	row := r.db.Pool.QueryRowContext(ctx, query, tokenValue, string(tType))

	var token, tokenType, userID string
	var expiresAt time.Time
	var usedAt sql.NullTime

	err := row.Scan(&token, &tokenType, &userID, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error al escanear token de verificacion: %w", err)
	}

	var tUsedAt *time.Time
	if usedAt.Valid {
		tUsedAt = &usedAt.Time
	}

	return domain.ReconstructVerificationToken(token, domain.TokenType(tokenType), userID, expiresAt, tUsedAt), nil
}

// UpdateVerificationToken marca el token como usado (UC-02/07)
func (r *PostgresUserRepository) UpdateVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	query := `UPDATE verification_tokens SET used_at = $1 WHERE token = $2`

	_, err := r.db.Pool.ExecContext(ctx, query, token.UsedAt(), token.Value())
	if err != nil {
		return fmt.Errorf("error al actualizar estado de uso del token: %w", err)
	}
	return nil
}

// Helper de infraestructura privado para encapsular la logica de des-mapeo de NullTimes a punteros nativos de Go
func (r *PostgresUserRepository) reconstructUser(id, email, status, phone string, phoneVerifiedAt sql.NullTime, createdAt time.Time) *domain.User {
	var verifiedAt *time.Time
	if phoneVerifiedAt.Valid {
		verifiedAt = &phoneVerifiedAt.Time
	}
	return domain.ReconstructUser(id, email, status, phone, verifiedAt, createdAt)
}
