package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestStartConversation_ErrorVerificandoExistente_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakeConversationRepo{
		findByPropertyAndParticipantsFn: func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
			return nil, boom
		},
	}
	uc := application.NewStartConversationUseCase(repo)

	_, err := uc.Execute(context.Background(), application.StartConversationCommand{
		PropertyID: "prop-1", SeekerID: "seeker-1", AdvertiserID: "advertiser-1",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestStartConversation_YaExisteUnHilo_LoDevuelveSinCrearOtro(t *testing.T) {
	existing := mustConversation(t, "seeker-1", "advertiser-1")
	repo := &fakeConversationRepo{
		findByPropertyAndParticipantsFn: func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
			return existing, nil
		},
	}
	uc := application.NewStartConversationUseCase(repo)

	dto, err := uc.Execute(context.Background(), application.StartConversationCommand{
		PropertyID: "prop-1", PropertyTitle: "Depto en Palermo",
		SeekerID: "seeker-1", SeekerName: "Juan", AdvertiserID: "advertiser-1", AdvertiserName: "Ana",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dto.ID != existing.ID() {
		t.Errorf("ID: got %q, want %q (el hilo existente)", dto.ID, existing.ID())
	}
	if repo.saved != nil {
		t.Error("Save: no debería invocarse cuando ya existe un hilo para esa propiedad y esos participantes")
	}
}

func TestStartConversation_DatosInvalidos_RetornaErrorDeDominio(t *testing.T) {
	// seeker == advertiser es rechazado por domain.NewConversation.
	repo := &fakeConversationRepo{
		findByPropertyAndParticipantsFn: func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
			return nil, nil
		},
	}
	uc := application.NewStartConversationUseCase(repo)

	_, err := uc.Execute(context.Background(), application.StartConversationCommand{
		PropertyID: "prop-1", SeekerID: "user-1", AdvertiserID: "user-1",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestStartConversation_ErrorAlPersistir_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("fallo de escritura")
	repo := &fakeConversationRepo{
		findByPropertyAndParticipantsFn: func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
			return nil, nil
		},
		saveErr: boom,
	}
	uc := application.NewStartConversationUseCase(repo)

	_, err := uc.Execute(context.Background(), application.StartConversationCommand{
		PropertyID: "prop-1", SeekerID: "seeker-1", AdvertiserID: "advertiser-1",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestStartConversation_HappyPath_CreaYPersisteUnaNuevaConversacion(t *testing.T) {
	repo := &fakeConversationRepo{
		findByPropertyAndParticipantsFn: func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
			return nil, nil
		},
	}
	uc := application.NewStartConversationUseCase(repo)

	dto, err := uc.Execute(context.Background(), application.StartConversationCommand{
		PropertyID: "prop-1", PropertyTitle: "Depto en Palermo",
		SeekerID: "seeker-1", SeekerName: "Juan", AdvertiserID: "advertiser-1", AdvertiserName: "Ana",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("Save: no fue invocado")
	}
	if dto.PropertyID != "prop-1" || dto.SeekerID != "seeker-1" || dto.AdvertiserID != "advertiser-1" {
		t.Errorf("ConversationDTO: got %+v", dto)
	}
	if dto.ID != repo.saved.ID() {
		t.Errorf("ID: got %q, want que coincida con la conversación persistida (%q)", dto.ID, repo.saved.ID())
	}
	// El caller es el seeker (cmd.SeekerID) — el "partner" debe ser el advertiser.
	if dto.PartnerName != "Ana" {
		t.Errorf("PartnerName: got %q, want %q (el advertiser)", dto.PartnerName, "Ana")
	}
	if dto.LastMessage != "" {
		t.Errorf("LastMessage: got %q, want vacío (no aplica al crear)", dto.LastMessage)
	}
}
