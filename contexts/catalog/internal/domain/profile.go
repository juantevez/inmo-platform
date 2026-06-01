package domain

import (
	"errors"
	"time"
)

// ProfileType define si es dueño directo o profesional inmobiliario
type ProfileType string

const (
	ProfileTypeIndividual ProfileType = "INDIVIDUAL"
	ProfileTypeCommercial ProfileType = "COMMERCIAL"
)

// ProfileStatus define el ciclo de vida del perfil comercial/dueño
type ProfileStatus string

const (
	StatusPending   ProfileStatus = "PENDING_VERIFICATION"
	StatusActive    ProfileStatus = "ACTIVE"
	StatusSuspended ProfileStatus = "SUSPENDED"
)

var (
	ErrInvalidProfileType    = errors.New("el tipo de perfil debe ser INDIVIDUAL o COMMERCIAL")
	ErrCommercialMissingData = errors.New("los perfiles comerciales requieren nombre de empresa y número de matrícula")
	ErrMissingRequiredFields = errors.New("nombre, apellido y dni/cuit son campos obligatorios")
)

// Profile representa el Agregado/Entidad del perfil de negocio en Catálogo
type Profile struct {
	userID        string
	firstName     string
	lastName      string
	dniCuit       string
	phone         string
	profileType   ProfileType
	companyName   string
	licenseNumber string
	status        ProfileStatus
	createdAt     time.Time
	updatedAt     time.Time
}

// NewProfile es el constructor de la entidad que valida invariantes de negocio
func NewProfile(
	userID, firstName, lastName, dniCuit, phone string,
	pType ProfileType,
	companyName, licenseNumber string,
) (*Profile, error) {

	if userID == "" || firstName == "" || lastName == "" || dniCuit == "" {
		return nil, ErrMissingRequiredFields
	}

	if pType != ProfileTypeIndividual && pType != ProfileTypeCommercial {
		return nil, ErrInvalidProfileType
	}

	// Regla de Negocio: Si es inmobiliaria/corredor, exigir matrícula y nombre comercial
	if pType == ProfileTypeCommercial {
		if companyName == "" || licenseNumber == "" {
			return nil, ErrCommercialMissingData
		}
	}

	// Si pasa las reglas, se crea con estado inicial verificado o pendiente según negocio
	now := time.Now()
	return &Profile{
		userID:        userID,
		firstName:     firstName,
		lastName:      lastName,
		dniCuit:       dniCuit,
		phone:         phone,
		profileType:   pType,
		companyName:   companyName,
		licenseNumber: licenseNumber,
		status:        StatusPending, // Arranca requiriendo validación
		createdAt:     now,
		updatedAt:     now,
	}, nil
}

// ReconstructProfile recrea la entidad desde los datos de infraestructura (Postgres) sin validar reglas de creación
func ReconstructProfile(
	userID, firstName, lastName, dniCuit, phone string,
	pType ProfileType,
	companyName, licenseNumber string,
	status ProfileStatus,
	createdAt, updatedAt time.Time,
) *Profile {
	return &Profile{
		userID:        userID,
		firstName:     firstName,
		lastName:      lastName,
		dniCuit:       dniCuit,
		phone:         phone,
		profileType:   pType,
		companyName:   companyName,
		licenseNumber: licenseNumber,
		status:        status,
		createdAt:     createdAt,
		updatedAt:     updatedAt,
	}
}

// Getters públicos para mantener el encapsulamiento de DDD
func (p *Profile) UserID() string           { return p.userID }
func (p *Profile) FirstName() string        { return p.firstName }
func (p *Profile) LastName() string         { return p.lastName }
func (p *Profile) DniCuit() string          { return p.dniCuit }
func (p *Profile) Phone() string            { return p.phone }
func (p *Profile) ProfileType() ProfileType { return p.profileType }
func (p *Profile) CompanyName() string      { return p.companyName }
func (p *Profile) LicenseNumber() string    { return p.licenseNumber }
func (p *Profile) Status() ProfileStatus    { return p.status }
func (p *Profile) CreatedAt() time.Time     { return p.createdAt }
func (p *Profile) UpdatedAt() time.Time     { return p.updatedAt }
