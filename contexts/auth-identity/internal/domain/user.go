package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEmail          = errors.New("el formato del email no es válido")
	ErrUserSuspended         = errors.New("el usuario se encuentra suspendido")
	ErrUserAlreadyActive     = errors.New("el usuario ya está activo")
	ErrProviderAlreadyLinked = errors.New("este método de autenticación ya está vinculado a la cuenta")
)

type UserStatus string

const (
	StatusPendingVerification UserStatus = "PENDING_VERIFICATION"
	StatusActive              UserStatus = "ACTIVE"
	StatusSuspended           UserStatus = "SUSPENDED"
)

// User representa el Agregado raíz del Bounded Context de Identidad
/*
type User struct {
	id              string
	email           string
	status          UserStatus
	phone           string
	phoneVerifiedAt *time.Time
	providers       []IdentityProvider
	createdAt       time.Time
}*/

type User struct {
	id              string
	email           string
	status          UserStatus
	phone           string
	phoneVerifiedAt *time.Time
	providers       []*IdentityProvider // ◄ Asegurate de que tenga el asterisco (*)
	createdAt       time.Time
}

// NewUser es el constructor del Agregado para un registro nuevo por Email/Password (UC-01)
func NewUser(id, email string) (*User, error) {
	if !strings.Contains(email, "@") { // Validación básica, después se puede usar regex
		return nil, ErrInvalidEmail
	}

	return &User{
		id:        id,
		email:     strings.ToLower(strings.TrimSpace(email)),
		status:    StatusPendingVerification,
		createdAt: time.Now(),
		providers: make([]*IdentityProvider, 0),
	}, nil
}

// NewUserFromSSO es el constructor para usuarios creados directo vía Google/Meta (UC-04/05)
func NewUserFromSSO(id, email string) (*User, error) {
	user, err := NewUser(id, email)
	if err != nil {
		return nil, err
	}
	// Al venir verificado por un tercero confiable, pasa directo a ACTIVE
	user.status = StatusActive
	return user, nil
}

// Activate pasa el estado del usuario a ACTIVE tras verificar el email (UC-02)
func (u *User) Activate() error {
	if u.status == StatusSuspended {
		return ErrUserSuspended
	}
	if u.status == StatusActive {
		return ErrUserAlreadyActive
	}

	u.status = StatusActive
	return nil
}

// LinkProvider ahora recibe un puntero (*IdentityProvider)
func (u *User) LinkProvider(newProvider *IdentityProvider) error { // ◄ Agregado el asterisco aca
	if u.status == StatusSuspended {
		return ErrUserSuspended
	}

	// Aplicamos tu sugerencia de seguridad: bloquear vinculación si no está verificado localmente
	if u.status == StatusPendingVerification && newProvider.Name() != ProviderEmail { // O "EMAIL"
		return errors.New("debe verificar su correo electrónico antes de vincular cuentas de SSO")
	}

	// Evitar duplicados en memoria (p ahora es un puntero)
	for _, p := range u.providers {
		if p.Name() == newProvider.Name() {
			return ErrProviderAlreadyLinked
		}
	}

	u.providers = append(u.providers, newProvider) // Guarda el puntero directo
	return nil
}

// ReconstructUser permite a la capa de infraestructura (Postgres) revivir un usuario existente
func ReconstructUser(id, email, status, phone string, phoneVerifiedAt *time.Time, createdAt time.Time) *User {
	return &User{
		id:              id,
		email:           email,
		status:          UserStatus(status), // o el tipo que use tu enum de estados
		phone:           phone,
		phoneVerifiedAt: phoneVerifiedAt,
		createdAt:       createdAt,
	}
}

// getters (Encapsulamiento estricto de DDD)
func (u *User) ID() string                     { return u.id }
func (u *User) Email() string                  { return u.email }
func (u *User) Status() UserStatus             { return u.status }
func (u *User) Phone() string                  { return u.phone }
func (u *User) PhoneVerifiedAt() *time.Time    { return u.phoneVerifiedAt }
func (u *User) Providers() []*IdentityProvider { return u.providers }
func (u *User) CreatedAt() time.Time           { return u.createdAt }
