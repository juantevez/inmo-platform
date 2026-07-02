package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// Reutiliza fakeConversationRepo/fakeMessageRepo (list_and_get_test.go) y
// fakeEventPublisher/fakeWebSocketHub/waitUntil (propose_visit_test.go) —
// mismo paquete application_test.

func newSendMessageUseCase(convRepo *fakeConversationRepo, msgRepo *fakeMessageRepo, hub *fakeWebSocketHub, publisher *fakeEventPublisher) *application.SendMessageUseCase {
	// db se pasa nil deliberadamente: SendMessageUseCase lo recibe en el
	// constructor pero Execute() nunca lo usa (ver nota en el test de abajo).
	return application.NewSendMessageUseCase(nil, convRepo, msgRepo, hub, publisher)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestSendMessage_ErrorBuscandoConversacion_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, boom },
	}
	uc := newSendMessageUseCase(convRepo, &fakeMessageRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: "conv-1", SenderID: "seeker-1", Body: "Hola"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestSendMessage_ConversacionNoExiste_RetornaNotFound(t *testing.T) {
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, nil },
	}
	uc := newSendMessageUseCase(convRepo, &fakeMessageRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: "no-existe", SenderID: "seeker-1", Body: "Hola"})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestSendMessage_SenderNoEsParticipante_RetornaForbidden(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	uc := newSendMessageUseCase(convRepo, &fakeMessageRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: conv.ID(), SenderID: "intruso-1", Body: "Hola"})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("Execute: got %v, want AppError Forbidden", err)
	}
}

func TestSendMessage_BodyVacio_RetornaErrorDeDominio(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	uc := newSendMessageUseCase(convRepo, &fakeMessageRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: conv.ID(), SenderID: "seeker-1", Body: ""})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (body vacío)", err)
	}
}

func TestSendMessage_ErrorGuardandoElMensaje_RetornaErrorEnvuelto(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	boom := errors.New("fallo de escritura")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	msgRepo := &fakeMessageRepo{saveErr: boom}
	uc := newSendMessageUseCase(convRepo, msgRepo, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: conv.ID(), SenderID: "seeker-1", Body: "Hola"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestSendMessage_ErrorActualizandoConversacion_SiPropagaElError(t *testing.T) {
	// A diferencia de ProposeVisitUseCase (que ignora el error de Touch/Save
	// con "_ ="), acá SendMessageUseCase SÍ propaga el error — el mensaje ya
	// fue persistido, pero el request completo falla igual. Inconsistencia
	// entre dos casos de uso muy similares, documentada por este test.
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	boom := errors.New("fallo al actualizar la conversación")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
		saveErr:    boom,
	}
	msgRepo := &fakeMessageRepo{}
	uc := newSendMessageUseCase(convRepo, msgRepo, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.SendMessageCommand{ConversationID: conv.ID(), SenderID: "seeker-1", Body: "Hola"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
	if msgRepo.saved == nil {
		t.Error("el mensaje debería haber quedado guardado pese al fallo posterior al tocar la conversación")
	}
}

func TestSendMessage_HappyPath_RetornaDTOYPublicaAsincrónicamente(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	publisher := &fakeEventPublisher{}
	uc := newSendMessageUseCase(convRepo, msgRepo, hub, publisher)

	dto, err := uc.Execute(context.Background(), application.SendMessageCommand{
		ConversationID: conv.ID(), SenderID: "advertiser-1", Body: "Sí, sigue disponible",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dto.ConversationID != conv.ID() || dto.SenderID != "advertiser-1" || dto.Body != "Sí, sigue disponible" {
		t.Errorf("MessageDTO: got %+v", dto)
	}
	if dto.Type != string(domain.MessageTypeText) {
		t.Errorf("Type: got %q, want %q", dto.Type, domain.MessageTypeText)
	}
	if msgRepo.saved == nil {
		t.Fatal("el mensaje debería haberse persistido")
	}
	if convRepo.saved == nil {
		t.Fatal("la conversación debería haberse tocado/persistido (Touch)")
	}

	waitUntil(t, time.Second, func() bool { return publisher.callCount() > 0 && hub.callCount() > 0 })

	pubCall := publisher.lastCall()
	if pubCall.subject != "chat.message.sent" {
		t.Errorf("Publish subject: got %q, want %q", pubCall.subject, "chat.message.sent")
	}

	bcCall := hub.lastCall()
	if bcCall.conversationID != conv.ID() {
		t.Errorf("Broadcast conversationID: got %q, want %q", bcCall.conversationID, conv.ID())
	}
	var wsMsg application.MessageDTO
	if err := json.Unmarshal(bcCall.payload, &wsMsg); err != nil {
		t.Fatalf("Broadcast payload: no es un MessageDTO válido: %v", err)
	}
	if wsMsg.ID != dto.ID || wsMsg.Body != dto.Body {
		t.Errorf("Broadcast MessageDTO: got %+v, want que coincida con el DTO retornado", wsMsg)
	}
}
