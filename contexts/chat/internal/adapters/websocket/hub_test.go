package websocket

// Test de caja blanca (mismo package, no "websocket_test"): Broadcast, register
// y unregister necesitan manipular el struct interno "client" y el canal
// "send" directamente para poder forzar deterministicamente el caso de
// "canal lleno → desconexión", que sería no determinístico si dependiéramos
// de una conexión TCP real y buffers del SO.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wsclient "golang.org/x/net/websocket"
)

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condición no se cumplió dentro del timeout")
}

func (h *Hub) roomExists(conversationID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.rooms[conversationID]
	return ok
}

func (h *Hub) roomSize(conversationID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[conversationID])
}

// ─── Broadcast / register / unregister (caja blanca) ───────────────────────

func TestHub_Broadcast_SinSalaRegistrada_NoOpea(t *testing.T) {
	h := NewHub()
	// No debe panickear ni bloquearse aunque nadie esté escuchando "conv-x".
	h.Broadcast("conv-x", []byte("hola"))
}

func TestHub_RegisterYBroadcast_EntregaAlCliente(t *testing.T) {
	h := NewHub()
	c := &client{userID: "user-1", conversationID: "conv-1", send: make(chan []byte, 4)}
	h.register(c)

	h.Broadcast("conv-1", []byte("hola mundo"))

	select {
	case got := <-c.send:
		if string(got) != "hola mundo" {
			t.Fatalf("payload: got %q, want %q", got, "hola mundo")
		}
	default:
		t.Fatal("esperaba un mensaje en el canal del cliente")
	}
}

func TestHub_Broadcast_SoloAlcanzaClientesDeLaMismaSala(t *testing.T) {
	h := NewHub()
	inRoom := &client{userID: "user-1", conversationID: "conv-1", send: make(chan []byte, 4)}
	otherRoom := &client{userID: "user-2", conversationID: "conv-2", send: make(chan []byte, 4)}
	h.register(inRoom)
	h.register(otherRoom)

	h.Broadcast("conv-1", []byte("solo para conv-1"))

	select {
	case <-inRoom.send:
	default:
		t.Fatal("el cliente de conv-1 debería haber recibido el mensaje")
	}
	select {
	case got := <-otherRoom.send:
		t.Fatalf("el cliente de conv-2 no debería recibir nada, recibió %q", got)
	default:
	}
}

func TestHub_Unregister_CierraCanalYLimpiaSalaVacia(t *testing.T) {
	h := NewHub()
	c := &client{userID: "user-1", conversationID: "conv-1", send: make(chan []byte, 4)}
	h.register(c)
	if !h.roomExists("conv-1") {
		t.Fatal("la sala debería existir tras register")
	}

	h.unregister(c)

	if h.roomExists("conv-1") {
		t.Fatal("la sala debería haberse eliminado al quedar vacía")
	}
	if _, ok := <-c.send; ok {
		t.Fatal("el canal send debería estar cerrado")
	}
}

func TestHub_Unregister_LlamadoDosVeces_EsIdempotente(t *testing.T) {
	h := NewHub()
	c := &client{userID: "user-1", conversationID: "conv-1", send: make(chan []byte, 4)}
	h.register(c)

	h.unregister(c)
	h.unregister(c) // no debe panickear por double-close del canal
}

func TestHub_Unregister_NoAfectaOtrosClientesDeLaMismaSala(t *testing.T) {
	h := NewHub()
	c1 := &client{userID: "user-1", conversationID: "conv-1", send: make(chan []byte, 4)}
	c2 := &client{userID: "user-2", conversationID: "conv-1", send: make(chan []byte, 4)}
	h.register(c1)
	h.register(c2)

	h.unregister(c1)

	if !h.roomExists("conv-1") {
		t.Fatal("la sala no debería eliminarse mientras quede un cliente")
	}
	if h.roomSize("conv-1") != 1 {
		t.Fatalf("esperaba 1 cliente restante, hay %d", h.roomSize("conv-1"))
	}
}

func TestHub_Unregister_ConexionViejaDelMismoUsuario_NoDesalojaALaNueva(t *testing.T) {
	// Regresión: si el mismo usuario reconecta (dos pestañas o reconexión
	// antes de que la vieja se cierre), register() sobreescribe la entrada
	// del map por userID. Cuando la conexión VIEJA finalmente dispara su
	// unregister() diferido, no debe desalojar a la conexión NUEVA que ya
	// ocupa esa clave.
	h := NewHub()
	old := &client{userID: "u1", conversationID: "conv-1", send: make(chan []byte, 4)}
	fresh := &client{userID: "u1", conversationID: "conv-1", send: make(chan []byte, 4)}
	h.register(old)
	h.register(fresh)

	h.unregister(old)

	if !h.roomExists("conv-1") {
		t.Fatal("la sala no debería eliminarse: 'fresh' sigue conectado")
	}
	if h.roomSize("conv-1") != 1 {
		t.Fatalf("esperaba que 'fresh' siga en la sala, quedaron %d clientes", h.roomSize("conv-1"))
	}
	if _, ok := <-old.send; ok {
		t.Fatal("el canal de la conexión vieja debería cerrarse igual")
	}
	select {
	case fresh.send <- []byte("sigue viva"):
	default:
		t.Fatal("el canal de 'fresh' no debería haberse cerrado")
	}
}

func TestHub_Broadcast_CanalLleno_DesconectaClienteLento(t *testing.T) {
	h := NewHub()
	// Canal de capacidad 1 sin lector — simula un cliente lento/bloqueado.
	slow := &client{userID: "user-lento", conversationID: "conv-1", send: make(chan []byte, 1)}
	h.register(slow)
	slow.send <- []byte("llenando el buffer")

	h.Broadcast("conv-1", []byte("este mensaje no entra"))

	if h.roomExists("conv-1") {
		t.Fatal("el hub debería haber desconectado al cliente lento y limpiado la sala")
	}
}

// ─── ServeWS ────────────────────────────────────────────────────────────────

func TestHub_ServeWS_FaltaConversationID_Retorna400(t *testing.T) {
	h := NewHub()
	handler := h.ServeWS("", "user-1")

	req := httptest.NewRequest(http.MethodGet, "/ws/chats/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHub_ServeWS_FaltaUserID_Retorna400(t *testing.T) {
	h := NewHub()
	handler := h.ServeWS("conv-1", "")

	req := httptest.NewRequest(http.MethodGet, "/ws/chats/conv-1", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHub_ServeWS_ConexionRealSeRegistraYRecibeBroadcast(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(h.ServeWS("conv-1", "user-1"))
	defer srv.Close()

	wsURL := "ws" + srv.URL[len("http"):] + "/"
	ws, err := wsclient.Dial(wsURL, "", srv.URL)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer ws.Close()

	waitUntil(t, 2*time.Second, func() bool { return h.roomSize("conv-1") == 1 })

	h.Broadcast("conv-1", []byte("mensaje de prueba"))

	if err := ws.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	var got string
	if err := wsclient.Message.Receive(ws, &got); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if got != "mensaje de prueba" {
		t.Fatalf("mensaje: got %q, want %q", got, "mensaje de prueba")
	}
}

func TestHub_ServeWS_ClienteDesconectaSeLimpiaLaSala(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(h.ServeWS("conv-1", "user-1"))
	defer srv.Close()

	wsURL := "ws" + srv.URL[len("http"):] + "/"
	ws, err := wsclient.Dial(wsURL, "", srv.URL)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool { return h.roomSize("conv-1") == 1 })

	ws.Close()

	waitUntil(t, 2*time.Second, func() bool { return !h.roomExists("conv-1") })
}
