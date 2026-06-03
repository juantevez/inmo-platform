package websocket

import (
	"log"
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

// client representa una conexión WebSocket activa de un usuario.
type client struct {
	userID         string
	conversationID string
	conn           *websocket.Conn
	send           chan []byte
}

// Hub mantiene el mapa de clientes activos y gestiona broadcast y desconexiones.
type Hub struct {
	mu      sync.RWMutex
	// rooms: conversationID → map[userID]*client
	rooms   map[string]map[string]*client
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[string]*client),
	}
}

// Broadcast envía el payload a todos los clientes suscritos a conversationID.
// Implementa ports.WebSocketHub.
func (h *Hub) Broadcast(conversationID string, payload []byte) {
	h.mu.RLock()
	room, ok := h.rooms[conversationID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Copiar punteros para no mantener el lock durante el envío
	clients := make([]*client, 0, len(room))
	for _, c := range room {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- payload:
		default:
			// Canal lleno — desconectar cliente lento
			log.Printf("[WS HUB] Canal lleno para user %s en conv %s, desconectando\n", c.userID, conversationID)
			h.unregister(c)
		}
	}
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[c.conversationID] == nil {
		h.rooms[c.conversationID] = make(map[string]*client)
	}
	h.rooms[c.conversationID][c.userID] = c
	log.Printf("[WS HUB] Usuario %s conectado a conversación %s\n", c.userID, c.conversationID)
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[c.conversationID]; ok {
		if _, exists := room[c.userID]; exists {
			delete(room, c.userID)
			close(c.send)
			log.Printf("[WS HUB] Usuario %s desconectado de conversación %s\n", c.userID, c.conversationID)
		}
		if len(room) == 0 {
			delete(h.rooms, c.conversationID)
		}
	}
}

// ServeWS es el http.HandlerFunc que hace el upgrade de la conexión.
// conversationID y userID deben inyectarse vía r.PathValue / header X-User-Id.
func (h *Hub) ServeWS(conversationID, userID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if conversationID == "" || userID == "" {
			http.Error(w, `{"error":"conversation_id y user_id son requeridos"}`, http.StatusBadRequest)
			return
		}

		wsHandler := websocket.Handler(func(conn *websocket.Conn) {
			c := &client{
				userID:         userID,
				conversationID: conversationID,
				conn:           conn,
				send:           make(chan []byte, 64),
			}
			h.register(c)
			defer h.unregister(c)

			// Goroutine de escritura
			go func() {
				for msg := range c.send {
					if err := websocket.Message.Send(conn, string(msg)); err != nil {
						log.Printf("[WS] Error enviando a %s: %v\n", userID, err)
						return
					}
				}
			}()

			// Bucle de lectura — mantiene la conexión viva y detecta desconexión
			var msg string
			for {
				if err := websocket.Message.Receive(conn, &msg); err != nil {
					break
				}
				// Por ahora descartamos mensajes entrantes por WS;
				// el envío se hace siempre vía REST POST /messages.
			}
		})

		wsHandler.ServeHTTP(w, r)
	}
}
