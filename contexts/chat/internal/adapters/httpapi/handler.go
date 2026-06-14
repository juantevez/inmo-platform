package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/shared/pkg/apperr"
)

type ChatHandler struct {
	startConvUC   *application.StartConversationUseCase
	listConvsUC   *application.ListConversationsUseCase
	getMessagesUC *application.GetMessagesUseCase
	sendMsgUC     *application.SendMessageUseCase
	proposeVisitUC *application.ProposeVisitUseCase
}

func NewChatHandler(
	startConvUC *application.StartConversationUseCase,
	listConvsUC *application.ListConversationsUseCase,
	getMessagesUC *application.GetMessagesUseCase,
	sendMsgUC *application.SendMessageUseCase,
	proposeVisitUC *application.ProposeVisitUseCase,
) *ChatHandler {
	return &ChatHandler{
		startConvUC:    startConvUC,
		listConvsUC:    listConvsUC,
		getMessagesUC:  getMessagesUC,
		sendMsgUC:      sendMsgUC,
		proposeVisitUC: proposeVisitUC,
	}
}

// POST /api/v1/chats
func (h *ChatHandler) HandleStartConversation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	var req struct {
		PropertyID     string `json:"property_id"`
		PropertyTitle  string `json:"property_title"`
		AdvertiserID   string `json:"advertiser_id"`
		AdvertiserName string `json:"advertiser_name"`
		SeekerName     string `json:"seeker_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	dto, err := h.startConvUC.Execute(r.Context(), application.StartConversationCommand{
		PropertyID:     req.PropertyID,
		PropertyTitle:  req.PropertyTitle,
		SeekerID:       userID,
		SeekerName:     req.SeekerName,
		AdvertiserID:   req.AdvertiserID,
		AdvertiserName: req.AdvertiserName,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	h.jsonResp(w, http.StatusCreated, dto)
}

// GET /api/v1/chats
func (h *ChatHandler) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	dtos, err := h.listConvsUC.Execute(r.Context(), userID)
	if err != nil {
		h.errResp(w, err)
		return
	}

	h.jsonResp(w, http.StatusOK, dtos)
}

// GET /api/v1/chats/{id}
func (h *ChatHandler) HandleGetMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	conversationID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	dtos, err := h.getMessagesUC.Execute(r.Context(), conversationID, userID, limit, offset)
	if err != nil {
		h.errResp(w, err)
		return
	}

	h.jsonResp(w, http.StatusOK, dtos)
}

// POST /api/v1/chats/{id}/messages
func (h *ChatHandler) HandleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	dto, err := h.sendMsgUC.Execute(r.Context(), application.SendMessageCommand{
		ConversationID: r.PathValue("id"),
		SenderID:       userID,
		Body:           req.Body,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	h.jsonResp(w, http.StatusCreated, dto)
}

// POST /api/v1/chats/{id}/visit-proposals
func (h *ChatHandler) HandleProposeVisit(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	var req struct {
		LeadID     string `json:"lead_id"`
		ProposedAt string `json:"proposed_at"` // RFC3339: "2026-07-15T10:00:00Z"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	proposedAt, err := time.Parse(time.RFC3339, req.ProposedAt)
	if err != nil {
		h.errResp(w, apperr.NewBadRequest("proposed_at inválido, usar formato RFC3339 (ej: 2026-07-15T10:00:00Z)", err))
		return
	}

	dto, err := h.proposeVisitUC.Execute(r.Context(), application.ProposeVisitCommand{
		ConversationID: r.PathValue("id"),
		SeekerID:       userID,
		LeadID:         req.LeadID,
		ProposedAt:     proposedAt,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	h.jsonResp(w, http.StatusCreated, dto)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func (h *ChatHandler) errResp(w http.ResponseWriter, err error) {
	code := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if ae, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(ae)
		return
	}
	_, _ = w.Write([]byte(`{"type":"INTERNAL_SERVER_ERROR","message":"error inesperado"}`))
}

func (h *ChatHandler) jsonResp(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
