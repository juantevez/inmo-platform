package application

import (
	"context"
	"errors"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

var ErrPropertyNotFound = errors.New("la propiedad especificada no existe o no está activa en mantenimiento")
var ErrPropertyClosed = errors.New("no se pueden abrir tickets en propiedades cerradas")

type CreateTicketCommand struct {
	ID          string `json:"id"`
	PropertyID  string `json:"property_id"`
	TenantID    string `json:"tenant_id"`
	Description string `json:"description"`
	Urgency     string `json:"urgency"` // "EMERGENCY", "URGENT", "SCHEDULED"
}

type CreateTicketUseCase struct {
	repo           ports.TicketRepository
	projectionRepo ports.PropertyProjectionRepository // FIX: usa proyección local en lugar del stub
	catalogService ports.CatalogService
}

func NewCreateTicketUseCase(
	repo ports.TicketRepository,
	projectionRepo ports.PropertyProjectionRepository,
	catalogService ports.CatalogService,
) *CreateTicketUseCase {
	return &CreateTicketUseCase{
		repo:           repo,
		projectionRepo: projectionRepo,
		catalogService: catalogService,
	}
}

func (uc *CreateTicketUseCase) Execute(ctx context.Context, cmd CreateTicketCommand) error {
	// 1. Verificar que la propiedad tiene proyección local (el evento de catalog ya llegó)
	//    Esto reemplaza la consulta directa a catalog — usa la tabla property_projections
	projection, err := uc.projectionRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return err
	}
	if projection == nil {
		// La proyección no existe: o la propiedad no fue publicada nunca,
		// o el evento catalog.property.published todavía no llegó (lag de NATS).
		// En ambos casos, no permitimos abrir el ticket.
		return ErrPropertyNotFound
	}

	// 2. Validar que la propiedad acepta tickets (no está CLOSED)
	if !projection.CanHaveActiveTicket() {
		return ErrPropertyClosed
	}

	// 3. Construir el Agregado en su estado inicial (OPEN)
	//    Usamos el tenant_id de la proyección si el comando no lo trae
	tenantID := cmd.TenantID
	if tenantID == "" {
		tenantID = projection.TenantID()
	}

	ticket := domain.NewTicket(
		cmd.ID,
		cmd.PropertyID,
		tenantID,
		cmd.Description,
		domain.UrgencyLevel(cmd.Urgency),
	)

	return uc.repo.Save(ctx, ticket)
}
