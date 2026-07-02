package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeConversationRepo struct {
	findByIDFn                      func(ctx context.Context, id string) (*domain.Conversation, error)
	findByParticipantFn             func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error)
	findByPropertyAndParticipantsFn func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error)
	saveErr                         error
	saved                           *domain.Conversation
}

func (f *fakeConversationRepo) Save(ctx context.Context, c *domain.Conversation) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = c
	return nil
}
func (f *fakeConversationRepo) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errors.New("FindByID: fixture no configurada")
}
func (f *fakeConversationRepo) FindByParticipant(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
	if f.findByParticipantFn != nil {
		return f.findByParticipantFn(ctx, userID)
	}
	return nil, errors.New("FindByParticipant: fixture no configurada")
}
func (f *fakeConversationRepo) FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
	if f.findByPropertyAndParticipantsFn != nil {
		return f.findByPropertyAndParticipantsFn(ctx, propertyID, seekerID, advertiserID)
	}
	return nil, errors.New("FindByPropertyAndParticipants: fixture no configurada")
}

type fakeMessageRepo struct {
	findByConversationFn func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error)
	saveErr              error
	saved                *domain.Message
}

func (f *fakeMessageRepo) Save(ctx context.Context, m *domain.Message) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = m
	return nil
}
func (f *fakeMessageRepo) FindByConversation(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
	if f.findByConversationFn != nil {
		return f.findByConversationFn(ctx, conversationID, limit, offset)
	}
	return nil, errors.New("FindByConversation: fixture no configurada")
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func mustConversation(t *testing.T, seekerID, advertiserID string) *domain.Conversation {
	t.Helper()
	c, err := domain.NewConversation("prop-1", "Depto en Palermo", seekerID, "Juan", advertiserID, "Ana")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}
	return c
}

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

// ─── ListConversationsUseCase ───────────────────────────────────────────────

func TestListConversations_UserIDVacio_RetornaBadRequest(t *testing.T) {
	uc := application.NewListConversationsUseCase(&fakeConversationRepo{})

	_, err := uc.Execute(context.Background(), "")
	assertBadRequest(t, err)
}

func TestListConversations_ErrorDelRepositorio_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakeConversationRepo{
		findByParticipantFn: func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) { return nil, boom },
	}
	uc := application.NewListConversationsUseCase(repo)

	_, err := uc.Execute(context.Background(), "user-1")

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestListConversations_SinResultados_RetornaListaVacia(t *testing.T) {
	repo := &fakeConversationRepo{
		findByParticipantFn: func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) { return nil, nil },
	}
	uc := application.NewListConversationsUseCase(repo)

	dtos, err := uc.Execute(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dtos == nil {
		t.Error("Execute: no debería devolver nil (se inicializa con make([]*ConversationDTO, 0, ...))")
	}
	if len(dtos) != 0 {
		t.Errorf("Execute: got %v, want lista vacía", dtos)
	}
}

func TestListConversations_HappyPath_PartnerNameSegunQuienConsulta(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1") // SeekerName=Juan, AdvertiserName=Ana
	repo := &fakeConversationRepo{
		findByParticipantFn: func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
			if userID != "seeker-1" {
				t.Errorf("FindByParticipant: got %q, want %q", userID, "seeker-1")
			}
			return []*ports.ConversationSummary{{Conversation: conv, LastMessage: "Hola, ¿sigue disponible?"}}, nil
		},
	}
	uc := application.NewListConversationsUseCase(repo)

	// Consulta como el SEEKER: el "partner" debe ser el advertiser (Ana).
	dtos, err := uc.Execute(context.Background(), "seeker-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("Execute: got %d DTOs, want 1", len(dtos))
	}
	dto := dtos[0]
	if dto.ID != conv.ID() || dto.PropertyID != "prop-1" || dto.SeekerID != "seeker-1" || dto.AdvertiserID != "advertiser-1" {
		t.Errorf("ConversationDTO: got %+v", dto)
	}
	if dto.PartnerName != "Ana" {
		t.Errorf("PartnerName: got %q, want %q (el advertiser, ya que consultamos como el seeker)", dto.PartnerName, "Ana")
	}
	if dto.LastMessage != "Hola, ¿sigue disponible?" {
		t.Errorf("LastMessage: got %q", dto.LastMessage)
	}
}

func TestListConversations_HappyPath_PartnerNameParaElAdvertiser(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	repo := &fakeConversationRepo{
		findByParticipantFn: func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
			return []*ports.ConversationSummary{{Conversation: conv, LastMessage: ""}}, nil
		},
	}
	uc := application.NewListConversationsUseCase(repo)

	// Consulta como el ADVERTISER: el "partner" debe ser el seeker (Juan).
	dtos, err := uc.Execute(context.Background(), "advertiser-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dtos[0].PartnerName != "Juan" {
		t.Errorf("PartnerName: got %q, want %q (el seeker, ya que consultamos como el advertiser)", dtos[0].PartnerName, "Juan")
	}
}

// ─── GetMessagesUseCase ─────────────────────────────────────────────────────

func newGetMessagesUseCase(convRepo ports.ConversationRepository, msgRepo ports.MessageRepository) *application.GetMessagesUseCase {
	return application.NewGetMessagesUseCase(convRepo, msgRepo)
}

func TestGetMessages_ErrorBuscandoConversacion_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, boom },
	}
	uc := newGetMessagesUseCase(convRepo, &fakeMessageRepo{})

	_, err := uc.Execute(context.Background(), "conv-1", "user-1", 50, 0)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestGetMessages_ConversacionNoExiste_RetornaNotFound(t *testing.T) {
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, nil },
	}
	uc := newGetMessagesUseCase(convRepo, &fakeMessageRepo{})

	_, err := uc.Execute(context.Background(), "no-existe", "user-1", 50, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestGetMessages_RequesterNoEsParticipante_RetornaForbidden(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	uc := newGetMessagesUseCase(convRepo, &fakeMessageRepo{})

	_, err := uc.Execute(context.Background(), conv.ID(), "intruso-1", 50, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("Execute: got %v, want AppError Forbidden", err)
	}
}

func TestGetMessages_LimiteFueraDeRango_CaeAlDefault(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}

	cases := []struct {
		name  string
		limit int
	}{
		{"cero", 0},
		{"negativo", -5},
		{"mayor a 100", 101},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedLimit int
			msgRepo := &fakeMessageRepo{
				findByConversationFn: func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
					capturedLimit = limit
					return nil, nil
				},
			}
			uc := newGetMessagesUseCase(convRepo, msgRepo)

			if _, err := uc.Execute(context.Background(), conv.ID(), "seeker-1", tc.limit, 0); err != nil {
				t.Fatalf("Execute: error inesperado: %v", err)
			}
			if capturedLimit != 50 {
				t.Errorf("limit con input %d: got %d, want 50 (default)", tc.limit, capturedLimit)
			}
		})
	}
}

func TestGetMessages_LimiteEnRangoValido_SePreserva(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}

	cases := []int{1, 25, 100}
	for _, limit := range cases {
		var capturedLimit, capturedOffset int
		msgRepo := &fakeMessageRepo{
			findByConversationFn: func(ctx context.Context, conversationID string, l, o int) ([]*domain.Message, error) {
				capturedLimit = l
				capturedOffset = o
				return nil, nil
			},
		}
		uc := newGetMessagesUseCase(convRepo, msgRepo)

		if _, err := uc.Execute(context.Background(), conv.ID(), "seeker-1", limit, 10); err != nil {
			t.Fatalf("Execute: error inesperado: %v", err)
		}
		if capturedLimit != limit {
			t.Errorf("limit: got %d, want %d (no debería aplicarse el default)", capturedLimit, limit)
		}
		if capturedOffset != 10 {
			t.Errorf("offset: got %d, want 10", capturedOffset)
		}
	}
}

func TestGetMessages_ErrorDelRepositorioDeMensajes_RetornaErrorEnvuelto(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	boom := errors.New("timeout de base de datos")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	msgRepo := &fakeMessageRepo{
		findByConversationFn: func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
			return nil, boom
		},
	}
	uc := newGetMessagesUseCase(convRepo, msgRepo)

	_, err := uc.Execute(context.Background(), conv.ID(), "seeker-1", 50, 0)

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestGetMessages_SinResultados_RetornaListaVacia(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	msgRepo := &fakeMessageRepo{
		findByConversationFn: func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
			return nil, nil
		},
	}
	uc := newGetMessagesUseCase(convRepo, msgRepo)

	dtos, err := uc.Execute(context.Background(), conv.ID(), "seeker-1", 50, 0)

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dtos == nil || len(dtos) != 0 {
		t.Errorf("Execute: got %v, want lista vacía no-nil", dtos)
	}
}

func TestGetMessages_HappyPath_MapeaTodosLosCampos(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	msg, err := domain.NewTextMessage(conv.ID(), "seeker-1", "Hola, ¿sigue disponible?")
	if err != nil {
		t.Fatalf("NewTextMessage: %v", err)
	}
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	msgRepo := &fakeMessageRepo{
		findByConversationFn: func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
			if conversationID != conv.ID() {
				t.Errorf("FindByConversation conversationID: got %q, want %q", conversationID, conv.ID())
			}
			return []*domain.Message{msg}, nil
		},
	}
	uc := newGetMessagesUseCase(convRepo, msgRepo)

	// El requester puede ser el advertiser (también es participante) aunque el mensaje sea del seeker.
	dtos, execErr := uc.Execute(context.Background(), conv.ID(), "advertiser-1", 50, 0)

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if len(dtos) != 1 {
		t.Fatalf("Execute: got %d DTOs, want 1", len(dtos))
	}
	dto := dtos[0]
	if dto.ID != msg.ID() || dto.ConversationID != conv.ID() || dto.SenderID != "seeker-1" ||
		dto.Body != "Hola, ¿sigue disponible?" || dto.Type != string(domain.MessageTypeText) {
		t.Errorf("MessageDTO: got %+v", dto)
	}
	wantCreatedAt := msg.CreatedAt().Format("2006-01-02T15:04:05Z")
	if dto.CreatedAt != wantCreatedAt {
		t.Errorf("CreatedAt: got %q, want %q", dto.CreatedAt, wantCreatedAt)
	}
}
