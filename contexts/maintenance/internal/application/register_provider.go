package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

var (
	ErrProviderAlreadyRegistered = errors.New("ya existe un proveedor registrado con ese user_id")
	ErrCuitCuilAlreadyExists     = errors.New("ya existe un proveedor registrado con ese CUIT/CUIL")
	ErrUnauthorizedRegistration  = errors.New("solo un ADMIN_INMO o el propio proveedor pueden realizar este registro")
)

// RegisterProviderCommand transporta los datos del formulario de alta.
//
// Hay dos flujos posibles según el rol del ejecutor:
//
//  1. AUTOREGISTRO (rol PROVEEDOR en el JWT):
//     - TargetUserID debe ser igual al CallerUserID (se registra a sí mismo)
//     - El sistema lo detecta automáticamente
//
//  2. REGISTRO POR ADMIN (rol ADMIN_INMO en el JWT):
//     - TargetUserID es el user_id del proveedor que se está dando de alta
//     - CallerUserID es el user_id del admin que ejecuta la acción
type RegisterProviderCommand struct {
	// Datos del proveedor a registrar
	ID                  string              `json:"id"`      // UUID generado por el caller
	TargetUserID        string              `json:"user_id"` // user_id del proveedor en auth_db
	RazonSocial         string              `json:"razon_social"`
	CuitCuil            string              `json:"cuit_cuil"`
	Rubro               domain.RubroTecnico `json:"rubro"`
	DisponibleUrgencias bool                `json:"disponible_urgencias"`

	// Datos opcionales de pago (pueden completarse después)
	CbuPago   string `json:"cbu_pago"`
	AliasPago string `json:"alias_pago"`

	// Contexto de autorización — se extrae del JWT en el handler, nunca del body
	CallerUserID string   `json:"-"`
	CallerRoles  []string `json:"-"`
}

// RegisterProviderResponse devuelve los datos del proveedor creado
type RegisterProviderResponse struct {
	ProviderID   string                `json:"provider_id"`
	UserID       string                `json:"user_id"`
	RazonSocial  string                `json:"razon_social"`
	Rubro        domain.RubroTecnico   `json:"rubro"`
	Status       domain.ProviderStatus `json:"status"`
	RegisteredBy string                `json:"registered_by"`
	CreatedAt    time.Time             `json:"created_at"`
}

type RegisterProviderUseCase struct {
	providerRepo ports.ProviderRepository
	uuidGen      func() string
}

func NewRegisterProviderUseCase(
	providerRepo ports.ProviderRepository,
	uuidGen func() string,
) *RegisterProviderUseCase {
	return &RegisterProviderUseCase{
		providerRepo: providerRepo,
		uuidGen:      uuidGen,
	}
}

func (uc *RegisterProviderUseCase) Execute(ctx context.Context, cmd RegisterProviderCommand) (*RegisterProviderResponse, error) {
	// 1. Validar autorización según el rol del caller
	//    - PROVEEDOR solo puede registrarse a sí mismo
	//    - ADMIN_INMO puede registrar a cualquiera
	if err := uc.validateAuthorization(cmd); err != nil {
		return nil, err
	}

	// 2. Verificar que el user_id no tenga ya un proveedor registrado
	existing, err := uc.providerRepo.FindByUserID(ctx, cmd.TargetUserID)
	if err != nil {
		return nil, fmt.Errorf("error al verificar existencia por user_id: %w", err)
	}
	if existing != nil {
		return nil, ErrProviderAlreadyRegistered
	}

	// 3. Verificar unicidad del CUIT/CUIL
	existingCuit, err := uc.providerRepo.FindByCuitCuil(ctx, cmd.CuitCuil)
	if err != nil {
		return nil, fmt.Errorf("error al verificar existencia por CUIT/CUIL: %w", err)
	}
	if existingCuit != nil {
		return nil, ErrCuitCuilAlreadyExists
	}

	// 4. Generar ID si el caller no lo proveyó
	providerID := cmd.ID
	if providerID == "" {
		providerID = uc.uuidGen()
	}

	// 5. Construir el agregado — el dominio valida rubro, CUIT y razón social
	provider, err := domain.NewProvider(
		providerID,
		cmd.TargetUserID,
		cmd.RazonSocial,
		cmd.CuitCuil,
		cmd.Rubro,
		cmd.DisponibleUrgencias,
		cmd.CallerUserID, // registeredBy: quién ejecutó el alta
	)
	if err != nil {
		return nil, err // errores de dominio: ErrInvalidRubro, ErrInvalidCuitCuil, etc.
	}

	// 6. Agregar datos de pago si vinieron en el comando (son opcionales al alta)
	if cmd.CbuPago != "" || cmd.AliasPago != "" {
		provider.SetDatosPago(cmd.CbuPago, cmd.AliasPago)
	}

	// 7. Persistir
	if err := uc.providerRepo.Save(ctx, provider); err != nil {
		return nil, fmt.Errorf("error al persistir el proveedor: %w", err)
	}

	return &RegisterProviderResponse{
		ProviderID:   provider.ID(),
		UserID:       provider.UserID(),
		RazonSocial:  provider.RazonSocial(),
		Rubro:        provider.Rubro(),
		Status:       provider.Status(),
		RegisteredBy: provider.RegisteredBy(),
		CreatedAt:    provider.CreatedAt(),
	}, nil
}

// validateAuthorization aplica las reglas de negocio de quién puede registrar a quién.
func (uc *RegisterProviderUseCase) validateAuthorization(cmd RegisterProviderCommand) error {
	isAdmin := hasRole(cmd.CallerRoles, "ADMIN_INMO")
	isProveedor := hasRole(cmd.CallerRoles, "PROVEEDOR")

	// Ni admin ni proveedor → rechazar directamente
	if !isAdmin && !isProveedor {
		return ErrUnauthorizedRegistration
	}

	// El proveedor solo puede registrarse a sí mismo
	if isProveedor && !isAdmin {
		if cmd.CallerUserID != cmd.TargetUserID {
			return errors.New("un proveedor solo puede registrar su propio perfil")
		}
	}

	// ADMIN_INMO puede registrar a cualquier user_id — sin restricción adicional

	return nil
}

// hasRole verifica si un rol específico está en la lista de roles del caller
func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
