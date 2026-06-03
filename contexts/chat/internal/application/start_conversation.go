package application

import (
	"context"
	"fmt"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
)

type StartConversationUseCase struct {
	convRepo ports.ConversationRepository
}

func NewStartConversationUseCase(convRepo ports.ConversationRepository) *StartConversationUseCase {
	return &StartConversationUseCase{convRepo: convRepo}
}

type StartConversationCommand struct {
	PropertyID   string
	SeekerID     string
	AdvertiserID string
}

type ConversationDTO struct {
	ID           string `json:"id"`
	PropertyID   string `json:"property_id"`
	SeekerID     string `json:"seeker_id"`
	AdvertiserID string `json:"advertiser_id"`
	LeadID       string `json:"lead_id,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (uc *StartConversationUseCase) Execute(ctx context.Context, cmd StartConversationCommand) (*ConversationDTO, error) {
	// Regla de negocio: si ya existe un hilo para esta propiedad entre estos dos usuarios, retornarlo.
	existing, err := uc.convRepo.FindByPropertyAndParticipants(ctx, cmd.PropertyID, cmd.SeekerID, cmd.AdvertiserID)
	if err != nil {
		return nil, fmt.Errorf("error al verificar conversación existente: %w", err)
	}
	if existing != nil {
		return toConversationDTO(existing), nil
	}

	conv, err := domain.NewConversation(cmd.PropertyID, cmd.SeekerID, cmd.AdvertiserID)
	if err != nil {
		return nil, err
	}

	if err := uc.convRepo.Save(ctx, conv); err != nil {
		return nil, fmt.Errorf("error al persistir la conversación: %w", err)
	}

	return toConversationDTO(conv), nil
}

func toConversationDTO(c *domain.Conversation) *ConversationDTO {
	return &ConversationDTO{
		ID:           c.ID(),
		PropertyID:   c.PropertyID(),
		SeekerID:     c.SeekerID(),
		AdvertiserID: c.AdvertiserID(),
		LeadID:       c.LeadID(),
		CreatedAt:    c.CreatedAt().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    c.UpdatedAt().Format("2006-01-02T15:04:05Z"),
	}
}
