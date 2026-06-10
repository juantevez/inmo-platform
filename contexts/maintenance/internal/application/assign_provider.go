package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

var (
	ErrTicketNotFound      = errors.New("el ticket de mantenimiento no existe")
	ErrProviderNotFound    = errors.New("el proveedor técnico no existe en el sistema")
	ErrProviderNotEligible = errors.New("el proveedor no está habilitado para recibir esta asignación")
)

type AssignProviderCommand struct {
	TicketID   string `json:"ticket_id"`
	ProviderID string `json:"provider_id"` // ID del agregado Provider (no el user_id)
}

type AssignProviderUseCase struct {
	repo         ports.TicketRepository
	providerRepo ports.ProviderRepository // FIX: ahora valida contra la tabla real
}

func NewAssignProviderUseCase(
	repo ports.TicketRepository,
	providerRepo ports.ProviderRepository,
) *AssignProviderUseCase {
	return &AssignProviderUseCase{
		repo:         repo,
		providerRepo: providerRepo,
	}
}

func (uc *AssignProviderUseCase) Execute(ctx context.Context, cmd AssignProviderCommand) error {
	// 1. Verificar que el ticket existe
	ticket, err := uc.repo.FindByID(ctx, cmd.TicketID)
	if err != nil {
		return fmt.Errorf("error al buscar el ticket: %w", err)
	}
	if ticket == nil {
		return ErrTicketNotFound
	}

	// 2. FIX: Verificar que el proveedor existe en proveedores_tecnicos
	//    Antes aceptaba cualquier string — ahora valida contra la tabla real
	provider, err := uc.providerRepo.FindByID(ctx, cmd.ProviderID)
	if err != nil {
		return fmt.Errorf("error al buscar el proveedor: %w", err)
	}
	if provider == nil {
		return ErrProviderNotFound
	}

	// 3. FIX: Validar que el proveedor puede recibir esta asignación
	//    El dominio verifica status (ACTIVE/SUSPENDED) y disponibilidad de urgencias
	if err := provider.CanReceiveAssignment(ticket.Urgency); err != nil {
		return fmt.Errorf("%w: %s", ErrProviderNotEligible, err.Error())
	}

	// 4. Mutación controlada por el dominio del ticket
	if err := ticket.AssignProvider(cmd.ProviderID); err != nil {
		return err // ErrInvalidStatusTransition si el ticket no está en OPEN
	}

	// 5. Persistir el ticket actualizado
	return uc.repo.Save(ctx, ticket)
}

// ListProvidersByRubroCommand permite al ADMIN consultar candidatos antes de asignar
type ListProvidersByRubroCommand struct {
	Rubro       domain.RubroTecnico
	OnlyUrgency bool // true = solo proveedores con disponible_urgencias = true
}

type ListProvidersUseCase struct {
	providerRepo ports.ProviderRepository
}

func NewListProvidersUseCase(providerRepo ports.ProviderRepository) *ListProvidersUseCase {
	return &ListProvidersUseCase{providerRepo: providerRepo}
}

func (uc *ListProvidersUseCase) Execute(ctx context.Context, cmd ListProvidersByRubroCommand) ([]*domain.Provider, error) {
	if cmd.OnlyUrgency {
		return uc.providerRepo.FindAvailableForEmergency(ctx, cmd.Rubro)
	}
	return uc.providerRepo.FindByRubro(ctx, cmd.Rubro)
}
