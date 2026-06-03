package application

import (
	"context"
	"encoding/json" // 🚀 Agregado para el Marshal del Outbox
	"fmt"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/contexts/crm/internal/ports"
)

type CreateAutoLeadDTO struct {
	PropertyID string
	OwnerID    string
}

type CreateAutoLeadUseCase struct {
	repo ports.LeadRepository
}

func NewCreateAutoLeadUseCase(repo ports.LeadRepository) *CreateAutoLeadUseCase {
	return &CreateAutoLeadUseCase{repo: repo}
}

func (uc *CreateAutoLeadUseCase) Execute(ctx context.Context, dto CreateAutoLeadDTO) error {
	// Generamos un ID interno único para el Lead de control
	leadID := fmt.Sprintf("lead-auto-%s", dto.PropertyID)

	// Creamos el lead asociándolo a la propiedad que nos llegó del evento
	// Usamos los campos correctos unificados en tu domain.Lead (ClientName)
	lead, err := domain.NewLead(
		leadID,
		dto.PropertyID,
		"SISTEMA - CAPTACION AUTOMATICA",
		"sistema@inmo-platform.com",
		"0800-INMO",
	)
	if err != nil {
		return err
	}

	// 🚀 1. Serializamos los datos del Lead a JSON para el payload del Outbox
	eventPayload, err := json.Marshal(map[string]interface{}{
		"id":          lead.ID,
		"property_id": lead.PropertyID,
		"client_name": lead.ClientName,
		"email":       lead.Email,
		"phone":       lead.Phone,
		"state":       string(lead.State),
		"owner_id":    dto.OwnerID, // Agregamos el OwnerID al evento por si el agente necesita saber de quién es
		"created_at":  lead.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("crm uc: error al serializar evento outbox: %w", err)
	}

	// 🚀 2. Persistimos el lead y el evento en la misma transacción pasando los 4 argumentos
	if err := uc.repo.Save(ctx, lead, "crm.lead.created", eventPayload); err != nil {
		return fmt.Errorf("crm uc: error al guardar el lead de forma transaccional: %w", err)
	}

	return nil
}
