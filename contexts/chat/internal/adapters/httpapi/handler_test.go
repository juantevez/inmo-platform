package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/adapters/httpapi"
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
	return nil, nil
}
func (f *fakeConversationRepo) FindByParticipant(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
	if f.findByParticipantFn != nil {
		return f.findByParticipantFn(ctx, userID)
	}
	return nil, nil
}
func (f *fakeConversationRepo) FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
	if f.findByPropertyAndParticipantsFn != nil {
		return f.findByPropertyAndParticipantsFn(ctx, propertyID, seekerID, advertiserID)
	}
	return nil, nil
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
	return nil, nil
}

type fakeVisitProposalRepo struct {
	saveErr error
	saved   *domain.VisitProposal
}

func (f *fakeVisitProposalRepo) Save(ctx context.Context, v *domain.VisitProposal) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = v
	return nil
}
func (f *fakeVisitProposalRepo) FindByID(ctx context.Context, id string) (*domain.VisitProposal, error) {
	return nil, errors.New("FindByID: no debería invocarse en estos tests")
}
func (f *fakeVisitProposalRepo) Update(ctx context.Context, v *domain.VisitProposal) error {
	return nil
}

type fakeWebSocketHub struct{}

func (f *fakeWebSocketHub) Broadcast(conversationID string, payload []byte) {}

type fakeEventPublisher struct{}

func (f *fakeEventPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	return nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

type testEnv struct {
	convRepo     *fakeConversationRepo
	msgRepo      *fakeMessageRepo
	proposalRepo *fakeVisitProposalRepo
}

func newTestHandler(env testEnv) *httpapi.ChatHandler {
	hub := &fakeWebSocketHub{}
	publisher := &fakeEventPublisher{}

	startConvUC := application.NewStartConversationUseCase(env.convRepo)
	listConvsUC := application.NewListConversationsUseCase(env.convRepo)
	getMessagesUC := application.NewGetMessagesUseCase(env.convRepo, env.msgRepo)
	sendMsgUC := application.NewSendMessageUseCase(nil, env.convRepo, env.msgRepo, hub, publisher)
	proposeVisitUC := application.NewProposeVisitUseCase(env.convRepo, env.msgRepo, env.proposalRepo, hub, publisher)

	return httpapi.NewChatHandler(startConvUC, listConvsUC, getMessagesUC, sendMsgUC, proposeVisitUC)
}

func newDefaultEnv() testEnv {
	return testEnv{
		convRepo:     &fakeConversationRepo{},
		msgRepo:      &fakeMessageRepo{},
		proposalRepo: &fakeVisitProposalRepo{},
	}
}

func mustConversation(t *testing.T, seekerID, advertiserID string) *domain.Conversation {
	t.Helper()
	c, err := domain.NewConversation("prop-1", "Depto en Palermo", seekerID, "Juan", advertiserID, "Ana")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}
	return c
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decodeBody: %v, body=%q", err, rec.Body.String())
	}
}

// ─── HandleStartConversation ────────────────────────────────────────────────

func TestHandleStartConversation_SinXUserId_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()

	h.HandleStartConversation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleStartConversation_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "seeker-1")
	rec := httptest.NewRecorder()

	h.HandleStartConversation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleStartConversation_ErrorDeDominio_Retorna400(t *testing.T) {
	// seeker (X-User-Id) == advertiser_id -> domain.NewConversation lo rechaza.
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"property_id": "prop-1", "advertiser_id": "user-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleStartConversation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleStartConversation_ErrorGenericoDelRepo_Retorna500(t *testing.T) {
	env := newDefaultEnv()
	env.convRepo.findByPropertyAndParticipantsFn = func(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
		return nil, errors.New("timeout de base de datos")
	}
	h := newTestHandler(env)
	body, _ := json.Marshal(map[string]string{"property_id": "prop-1", "advertiser_id": "advertiser-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	rec := httptest.NewRecorder()

	h.HandleStartConversation(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleStartConversation_HappyPath_Retorna201(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{
		"property_id": "prop-1", "property_title": "Depto en Palermo",
		"advertiser_id": "advertiser-1", "advertiser_name": "Ana", "seeker_name": "Juan",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	rec := httptest.NewRecorder()

	h.HandleStartConversation(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var dto application.ConversationDTO
	decodeBody(t, rec, &dto)
	if dto.SeekerID != "seeker-1" || dto.AdvertiserID != "advertiser-1" {
		t.Errorf("ConversationDTO: got %+v", dto)
	}
}

// ─── HandleListConversations ────────────────────────────────────────────────

func TestHandleListConversations_SinXUserId_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats", nil)
	rec := httptest.NewRecorder()

	h.HandleListConversations(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListConversations_ErrorGenericoDelRepo_Retorna500(t *testing.T) {
	env := newDefaultEnv()
	env.convRepo.findByParticipantFn = func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
		return nil, errors.New("timeout de base de datos")
	}
	h := newTestHandler(env)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	rec := httptest.NewRecorder()

	h.HandleListConversations(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleListConversations_HappyPath_Retorna200(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByParticipantFn = func(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
		return []*ports.ConversationSummary{{Conversation: conv, LastMessage: "Hola"}}, nil
	}
	h := newTestHandler(env)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	rec := httptest.NewRecorder()

	h.HandleListConversations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var dtos []application.ConversationDTO
	decodeBody(t, rec, &dtos)
	if len(dtos) != 1 || dtos[0].ID != conv.ID() {
		t.Errorf("ConversationDTOs: got %+v", dtos)
	}
}

// ─── HandleGetMessages ──────────────────────────────────────────────────────

func TestHandleGetMessages_SinXUserId_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/conv-1", nil)
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGetMessages_ConversacionNoExiste_Retorna404(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/no-existe", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetMessages_NoEsParticipante_Retorna403(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	h := newTestHandler(env)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/"+conv.ID(), nil)
	req.Header.Set("X-User-Id", "intruso-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleGetMessages_LimitInvalidoEnQuery_NoRompeYUsaDefault(t *testing.T) {
	// strconv.Atoi("abc") falla y el handler ignora el error -> limit queda en 0,
	// que GetMessagesUseCase interpreta como "usar default" (50).
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	var capturedLimit int
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	env.msgRepo.findByConversationFn = func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
		capturedLimit = limit
		return nil, nil
	}
	h := newTestHandler(env)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/"+conv.ID()+"?limit=abc", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if capturedLimit != 50 {
		t.Errorf("limit: got %d, want 50 (default aplicado por el use case)", capturedLimit)
	}
}

func TestHandleGetMessages_HappyPath_Retorna200(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	msg, err := domain.NewTextMessage(conv.ID(), "seeker-1", "Hola")
	if err != nil {
		t.Fatalf("NewTextMessage: %v", err)
	}
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	env.msgRepo.findByConversationFn = func(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
		return []*domain.Message{msg}, nil
	}
	h := newTestHandler(env)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/"+conv.ID()+"?limit=10&offset=0", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var dtos []application.MessageDTO
	decodeBody(t, rec, &dtos)
	if len(dtos) != 1 || dtos[0].ID != msg.ID() {
		t.Errorf("MessageDTOs: got %+v", dtos)
	}
}

// ─── HandleSendMessage ──────────────────────────────────────────────────────

func TestHandleSendMessage_SinXUserId_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/conv-1/messages", bytes.NewBufferString("{}"))
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendMessage_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/conv-1/messages", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendMessage_BodyVacio_Retorna400(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	h := newTestHandler(env)
	body, _ := json.Marshal(map[string]string{"body": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/"+conv.ID()+"/messages", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleSendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSendMessage_HappyPath_Retorna201(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	h := newTestHandler(env)
	body, _ := json.Marshal(map[string]string{"body": "Hola, ¿sigue disponible?"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/"+conv.ID()+"/messages", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleSendMessage(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var dto application.MessageDTO
	decodeBody(t, rec, &dto)
	if dto.Body != "Hola, ¿sigue disponible?" || dto.SenderID != "seeker-1" {
		t.Errorf("MessageDTO: got %+v", dto)
	}
}

// ─── HandleProposeVisit ─────────────────────────────────────────────────────

func TestHandleProposeVisit_SinXUserId_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/conv-1/visit-proposals", bytes.NewBufferString("{}"))
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleProposeVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleProposeVisit_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/conv-1/visit-proposals", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleProposeVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleProposeVisit_ProposedAtNoEsRFC3339_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"lead_id": "lead-1", "proposed_at": "15/07/2026"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/conv-1/visit-proposals", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", "conv-1")
	rec := httptest.NewRecorder()

	h.HandleProposeVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var got apperr.AppError
	decodeBody(t, rec, &got)
	if got.Message == "" {
		t.Error("Message: no debería estar vacío")
	}
}

func TestHandleProposeVisit_FechaEnElPasado_Retorna400(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	h := newTestHandler(env)
	body, _ := json.Marshal(map[string]string{"lead_id": "lead-1", "proposed_at": "2020-01-01T10:00:00Z"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/"+conv.ID()+"/visit-proposals", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleProposeVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleProposeVisit_HappyPath_Retorna201(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	env := newDefaultEnv()
	env.convRepo.findByIDFn = func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil }
	h := newTestHandler(env)

	proposedAt := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	body, _ := json.Marshal(map[string]string{"lead_id": "lead-1", "proposed_at": proposedAt})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chats/"+conv.ID()+"/visit-proposals", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", conv.ID())
	rec := httptest.NewRecorder()

	h.HandleProposeVisit(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var dto application.VisitProposalDTO
	decodeBody(t, rec, &dto)
	if dto.ConversationID != conv.ID() || dto.LeadID != "lead-1" {
		t.Errorf("VisitProposalDTO: got %+v", dto)
	}
}

// ─── errResp: estructura del AppError expuesta al cliente ───────────────────

func TestErrResp_AppError_ExponeTypeYMessage(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chats/no-existe", nil)
	req.Header.Set("X-User-Id", "seeker-1")
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.HandleGetMessages(rec, req)

	var got apperr.AppError
	decodeBody(t, rec, &got)
	if got.Type != apperr.TypeNotFound {
		t.Errorf("Type: got %q, want %q", got.Type, apperr.TypeNotFound)
	}
}
