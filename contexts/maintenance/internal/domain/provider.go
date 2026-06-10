package domain

import (
	"errors"
	"strings"
	"time"
)

// RubroTecnico representa el tipo de trabajo que realiza el proveedor.
// Debe coincidir exactamente con el ENUM rubro_tecnico de Postgres.
type RubroTecnico string

const (
	RubroPlomero           RubroTecnico = "PLOMERO"
	RubroElectricista      RubroTecnico = "ELECTRICISTA"
	RubroAlbanil           RubroTecnico = "ALBANIL"
	RubroGasista           RubroTecnico = "GASISTA"
	RubroCerrajero         RubroTecnico = "CERRAJERO"
	RubroPintor            RubroTecnico = "PINTOR"
	RubroCarpintero        RubroTecnico = "CARPINTERO"
	RubroHerrero           RubroTecnico = "HERRERO"
	RubroTechista          RubroTecnico = "TECHISTA"
	RubroJardinero         RubroTecnico = "JARDINERO"
	RubroAireAcondicionado RubroTecnico = "AIRE_ACONDICIONADO"
	RubroAscensores        RubroTecnico = "ASCENSORES"
	RubroLimpieza          RubroTecnico = "LIMPIEZA"
	RubroOtros             RubroTecnico = "OTROS"
)

// ProviderStatus representa el estado del ciclo de vida del proveedor en el sistema
type ProviderStatus string

const (
	ProviderStatusActive    ProviderStatus = "ACTIVE"
	ProviderStatusSuspended ProviderStatus = "SUSPENDED"
	ProviderStatusInactive  ProviderStatus = "INACTIVE"
)

// Errores de dominio del agregado Provider
var (
	ErrInvalidRubro          = errors.New("el rubro técnico especificado no es válido")
	ErrProviderSuspended     = errors.New("el proveedor se encuentra suspendido y no puede recibir asignaciones")
	ErrProviderInactive      = errors.New("el proveedor está inactivo")
	ErrInvalidCuitCuil       = errors.New("el CUIT/CUIL debe tener 11 dígitos numéricos")
	ErrRazonSocialEmpty      = errors.New("la razón social o nombre del proveedor no puede estar vacío")
	ErrProviderAlreadyActive = errors.New("el proveedor ya se encuentra activo")
)

// rubrosValidos es el conjunto de valores aceptados — espejo del ENUM de Postgres
var rubrosValidos = map[RubroTecnico]bool{
	RubroPlomero:           true,
	RubroElectricista:      true,
	RubroAlbanil:           true,
	RubroGasista:           true,
	RubroCerrajero:         true,
	RubroPintor:            true,
	RubroCarpintero:        true,
	RubroHerrero:           true,
	RubroTechista:          true,
	RubroJardinero:         true,
	RubroAireAcondicionado: true,
	RubroAscensores:        true,
	RubroLimpieza:          true,
	RubroOtros:             true,
}

// Provider es el Agregado que representa a un técnico o empresa de mantenimiento.
// Su user_id es el vínculo lógico con auth_db — no hay FK física entre bases.
type Provider struct {
	id                  string
	userID              string // vínculo con auth_db.users
	razonSocial         string
	cuitCuil            string
	rubro               RubroTecnico
	cbuPago             string
	aliasPago           string
	disponibleUrgencias bool
	status              ProviderStatus
	registeredBy        string // user_id de quien lo registró (ADMIN o él mismo)
	createdAt           time.Time
	updatedAt           time.Time
}

// NewProvider es el constructor del agregado con todas las validaciones de negocio.
// registeredBy es el user_id de quien ejecuta la acción (puede ser el mismo proveedor o un ADMIN_INMO).
func NewProvider(id, userID, razonSocial, cuitCuil string, rubro RubroTecnico, disponibleUrgencias bool, registeredBy string) (*Provider, error) {
	if strings.TrimSpace(razonSocial) == "" {
		return nil, ErrRazonSocialEmpty
	}

	if !isValidCuitCuil(cuitCuil) {
		return nil, ErrInvalidCuitCuil
	}

	if !rubrosValidos[rubro] {
		return nil, ErrInvalidRubro
	}

	now := time.Now()
	return &Provider{
		id:                  id,
		userID:              userID,
		razonSocial:         strings.TrimSpace(razonSocial),
		cuitCuil:            cuitCuil,
		rubro:               rubro,
		disponibleUrgencias: disponibleUrgencias,
		status:              ProviderStatusActive, // nace ACTIVE — ya fue validado por quien lo registra
		registeredBy:        registeredBy,
		createdAt:           now,
		updatedAt:           now,
	}, nil
}

// ReconstructProvider rehidrata el agregado desde Postgres sin validaciones de creación.
// Solo lo usa la capa de infraestructura (repositorio).
func ReconstructProvider(
	id, userID, razonSocial, cuitCuil string,
	rubro RubroTecnico,
	cbuPago, aliasPago string,
	disponibleUrgencias bool,
	status ProviderStatus,
	registeredBy string,
	createdAt, updatedAt time.Time,
) *Provider {
	return &Provider{
		id:                  id,
		userID:              userID,
		razonSocial:         razonSocial,
		cuitCuil:            cuitCuil,
		rubro:               rubro,
		cbuPago:             cbuPago,
		aliasPago:           aliasPago,
		disponibleUrgencias: disponibleUrgencias,
		status:              status,
		registeredBy:        registeredBy,
		createdAt:           createdAt,
		updatedAt:           updatedAt,
	}
}

// SetDatosPago actualiza los datos de cobro del proveedor (CBU/Alias).
// Son opcionales al registro pero necesarios antes de recibir pagos.
func (p *Provider) SetDatosPago(cbu, alias string) {
	p.cbuPago = cbu
	p.aliasPago = alias
	p.updatedAt = time.Now()
}

// Suspend inhabilita al proveedor para recibir nuevas asignaciones.
func (p *Provider) Suspend() error {
	if p.status == ProviderStatusSuspended {
		return errors.New("el proveedor ya está suspendido")
	}
	p.status = ProviderStatusSuspended
	p.updatedAt = time.Now()
	return nil
}

// Reactivate vuelve a habilitar un proveedor suspendido o inactivo.
func (p *Provider) Reactivate() error {
	if p.status == ProviderStatusActive {
		return ErrProviderAlreadyActive
	}
	p.status = ProviderStatusActive
	p.updatedAt = time.Now()
	return nil
}

// CanReceiveAssignment valida si el proveedor está habilitado para recibir un ticket.
// Para tickets EMERGENCY, también verifica la disponibilidad de urgencias.
func (p *Provider) CanReceiveAssignment(urgency UrgencyLevel) error {
	if p.status == ProviderStatusSuspended {
		return ErrProviderSuspended
	}
	if p.status == ProviderStatusInactive {
		return ErrProviderInactive
	}
	if urgency == UrgencyEmergency && !p.disponibleUrgencias {
		return errors.New("el proveedor no acepta tickets de emergencia fuera de horario")
	}
	return nil
}

// Getters — encapsulamiento estricto de DDD
func (p *Provider) ID() string                { return p.id }
func (p *Provider) UserID() string            { return p.userID }
func (p *Provider) RazonSocial() string       { return p.razonSocial }
func (p *Provider) CuitCuil() string          { return p.cuitCuil }
func (p *Provider) Rubro() RubroTecnico       { return p.rubro }
func (p *Provider) CbuPago() string           { return p.cbuPago }
func (p *Provider) AliasPago() string         { return p.aliasPago }
func (p *Provider) DisponibleUrgencias() bool { return p.disponibleUrgencias }
func (p *Provider) Status() ProviderStatus    { return p.status }
func (p *Provider) RegisteredBy() string      { return p.registeredBy }
func (p *Provider) CreatedAt() time.Time      { return p.createdAt }
func (p *Provider) UpdatedAt() time.Time      { return p.updatedAt }

// isValidCuitCuil valida que el CUIT/CUIL tenga exactamente 11 dígitos numéricos
func isValidCuitCuil(cuit string) bool {
	if len(cuit) != 11 {
		return false
	}
	for _, c := range cuit {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
