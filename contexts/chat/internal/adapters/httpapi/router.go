package httpapi

import (
	"net/http"

	"inmo.platform/contexts/chat/internal/adapters/websocket"
	"inmo.platform/shared/pkg/health"
)

func NewRouter(h *ChatHandler, hub *websocket.Hub, checker *health.Checker) http.Handler {
	mux := http.NewServeMux()

	// Conversaciones
	mux.HandleFunc("POST /api/v1/chats", h.HandleStartConversation)
	mux.HandleFunc("GET /api/v1/chats", h.HandleListConversations)
	mux.HandleFunc("GET /api/v1/chats/{id}", h.HandleGetMessages)

	// Mensajes
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", h.HandleSendMessage)

	// Propuestas de visita
	mux.HandleFunc("POST /api/v1/chats/{id}/visit-proposals", h.HandleProposeVisit)

	// WebSocket — el gateway hace el upgrade y reenvía al handler
	// El userID viene del header X-User-Id inyectado por el authMiddleware del gateway
	mux.HandleFunc("GET /ws/chats/{id}", func(w http.ResponseWriter, r *http.Request) {
		conversationID := r.PathValue("id")
		userID := r.Header.Get("X-User-Id")
		hub.ServeWS(conversationID, userID)(w, r)
	})

	// Health checks
	mux.HandleFunc("GET /health/live", checker.LiveHandler)
	mux.HandleFunc("GET /health/ready", checker.ReadyHandler)

	return mux
}
