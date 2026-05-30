package domain

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidProvider = errors.New("el proveedor de identidad no es válido")
	ErrPasswordTooWeak = errors.New("la contraseña debe tener al menos 8 caracteres, una mayúscula y un número")
	ErrEmptyProviderID = errors.New("el ID del proveedor externo no puede estar vacío")
)

type ProviderType string

const (
	ProviderEmail  ProviderType = "EMAIL"
	ProviderGoogle ProviderType = "GOOGLE"
	ProviderMeta   ProviderType = "META"
)

// IdentityProvider es el struct ÚNICO que entiende tu puerto/repositorio
type IdentityProvider struct {
	id             string
	userID         string
	providerName   ProviderType
	providerUserID string // Email para tradicional, sub para Google, id para Meta
	passwordHash   string // Puede ser vacío si entra por SSO
}

// NewEmailProvider ahora retorna un *IdentityProvider genérico pero preconfigurado
func NewEmailProvider(id string, userID string, email string, plainPassword string) (*IdentityProvider, error) {
	// 1. Validaciones de negocio específicas para el flujo tradicionales
	if len(plainPassword) < 8 {
		return nil, errors.New("la contraseña debe tener al menos 8 caracteres, una mayúscula y un número")
	}
	// TODO: Agregar acá tus chequeos de Regex para la mayúscula y el número si no los tenías

	// 2. Hashear la contraseña con costo 12 (Regla de seguridad del dominio)
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(plainPassword), 12)
	if err != nil {
		return nil, err
	}

	// 3. Retornamos el tipo base IdentityProvider que exige el puerto
	return &IdentityProvider{
		id:             id,
		userID:         userID,
		providerName:   ProviderEmail,
		providerUserID: email,
		passwordHash:   string(hashedBytes),
	}, nil
}

// ReconstructProvider reconstruye un IdentityProvider desde la base de datos sin validaciones de creación
func ReconstructProvider(id, userID string, pType ProviderType, providerUserID, passwordHash string) *IdentityProvider {
	return &IdentityProvider{
		id:             id,
		userID:         userID,
		providerName:   pType,
		providerUserID: providerUserID,
		passwordHash:   passwordHash,
	}
}

// NewSSOProvider corregido: Usa providerName e incluye el userID mandatorio
func NewSSOProvider(id string, userID string, pType ProviderType, providerUserID string) (*IdentityProvider, error) {
	if pType != ProviderGoogle && pType != ProviderMeta {
		return nil, ErrInvalidProvider
	}
	if providerUserID == "" {
		return nil, ErrEmptyProviderID
	}

	return &IdentityProvider{
		id:             id,
		userID:         userID, // ◄ Agregado para cumplir la FK de tu DB
		providerName:   pType,  // ◄ Sincronizado con el nombre del campo del struct
		providerUserID: providerUserID,
		passwordHash:   "", // SSO no maneja contraseña local
	}, nil
}

// VerifyPassword corregido: Apunta a p.providerName
func (p *IdentityProvider) VerifyPassword(password string) bool {
	if p.providerName != ProviderEmail { // ◄ Cambiado a providerName
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(p.passwordHash), []byte(password))
	return err == nil
}

// PasswordHash es el Getter que necesita el repositorio para guardar el hash en Postgres
func (p *IdentityProvider) PasswordHash() string {
	return p.passwordHash
}

// SetPasswordHash es el Setter que necesita el repositorio al reconstruir el objeto desde la DB
func (p *IdentityProvider) SetPasswordHash(hash string) {
	p.passwordHash = hash
}

// Getters de encapsulamiento
func (p *IdentityProvider) ID() string             { return p.id }
func (p *IdentityProvider) Name() ProviderType     { return p.providerName }
func (p *IdentityProvider) ProviderUserID() string { return p.providerUserID }
