package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
)

var (
	ErrDniCuitAlreadyExists = errors.New("el número de documento o CUIT ya se encuentra registrado")
)

// CreateProfileCommand transporta los datos crudos desde la capa de entrada (HTTP)
type CreateProfileCommand struct {
	UserID        string
	FirstName     string
	LastName      string
	DniCuit       string
	Phone         string
	ProfileType   string // "INDIVIDUAL" o "COMMERCIAL"
	CompanyName   string // Opcional, solo comercial
	LicenseNumber string // Opcional, solo comercial
}

type CreateProfileUseCase struct {
	profileRepo ports.ProfileRepository
}

func NewCreateProfileUseCase(profileRepo ports.ProfileRepository) *CreateProfileUseCase {
	return &CreateProfileUseCase{profileRepo: profileRepo}
}

func (uc *CreateProfileUseCase) Execute(ctx context.Context, cmd CreateProfileCommand) error {
	// 1. Validar si ya existe un perfil con ese mismo DNI o CUIT (Regla de negocio macro)
	existingProfile, err := uc.profileRepo.FindByDniCuit(ctx, cmd.DniCuit)
	if err != nil {
		return fmt.Errorf("error al verificar unicidad de documento: %w", err)
	}

	// Si el perfil existe y pertenece a OTRO usuario, tiramos error de conflicto de negocio
	if existingProfile != nil && existingProfile.UserID() != cmd.UserID {
		return ErrDniCuitAlreadyExists
	}

	// 2. Instanciar el Agregado de Dominio aplicando las invariantes de validación
	profile, err := domain.NewProfile(
		cmd.UserID,
		cmd.FirstName,
		cmd.LastName,
		cmd.DniCuit,
		cmd.Phone,
		domain.ProfileType(cmd.ProfileType),
		cmd.CompanyName,
		cmd.LicenseNumber,
	)
	if err != nil {
		return fmt.Errorf("error de validación de dominio: %w", err) // Devuelve los errores finos del dominio
	}

	// 3. Persistir en la base de datos de catálogo (inmo_catalog_db)
	err = uc.profileRepo.Save(ctx, profile)
	if err != nil {
		return fmt.Errorf("error al persistir el perfil en el repositorio: %w", err)
	}

	return nil
}
