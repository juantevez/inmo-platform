package nats_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	natsadapter "inmo.platform/contexts/contracts/internal/adapters/nats"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// ReservationNotificationSubscriber lee GATEWAY_URL de una variable de entorno
// al construirse (no es inyectable), así que apuntamos ese gateway a un
// httptest.NewServer que simula el chat service.

type recordedRequest struct {
	method string
	path   string
	header http.Header
	body   map[string]any
}

// fakeChatGateway simula POST /api/v1/chats (resolveChat) y
// POST /api/v1/chats/{id}/messages (postSystemMessage).
type fakeChatGateway struct {
	mu            sync.Mutex
	requests      []recordedRequest
	resolveStatus int
	resolveBody   string // si vacío, responde {"id":"chat-1"}
	messageStatus int
}

func newFakeChatGateway() *fakeChatGateway {
	return &fakeChatGateway{resolveStatus: http.StatusOK, messageStatus: http.StatusOK}
}

func (g *fakeChatGateway) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/chats", func(w http.ResponseWriter, r *http.Request) {
		g.record(t, r)
		if g.resolveStatus != http.StatusOK {
			w.WriteHeader(g.resolveStatus)
			return
		}
		body := g.resolveBody
		if body == "" {
			body = `{"id":"chat-1"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		g.record(t, r)
		w.WriteHeader(g.messageStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (g *fakeChatGateway) record(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]any
	data, _ := io.ReadAll(r.Body)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &body)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, recordedRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body})
}

func (g *fakeChatGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

func (g *fakeChatGateway) at(i int) recordedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.requests[i]
}

// countPath cuenta cuántas requests registradas fueron al path dado. Se usa en
// vez de aserciones de conteo exacto porque un Nak() sin delay explícito
// dispara redelivery casi inmediata en el servidor NATS embebido (no respeta
// el BackOff configurado a nivel de consumer de la forma en que uno
// esperaría), así que el número total de reintentos observados no es
// determinístico dentro de la ventana de espera del test.
func (g *fakeChatGateway) countPath(path string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, r := range g.requests {
		if r.path == path {
			n++
		}
	}
	return n
}

func newContractsStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     "contracts",
		Subjects: []string{"contracts.>"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestReservationNotificationSubscriber_ReservaConfirmada_NotificaAlChat(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	payload := `{
		"aggregate_id": "res-1",
		"property_id": "prop-1",
		"tenant_id": "tenant-1",
		"owner_id": "owner-1",
		"check_in_date": "2026-08-10",
		"check_out_date": "2026-08-15",
		"total_amount": 45000
	}`
	publishJSON(t, js, "contracts.reservation.confirmed", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return gw.count() >= 2 }, 3*time.Second)

	if gw.count() != 2 {
		t.Fatalf("esperaba 2 llamadas HTTP (resolve + message), hubo %d", gw.count())
	}
	resolve := gw.at(0)
	if resolve.path != "/api/v1/chats" {
		t.Fatalf("primera llamada: got %s, want /api/v1/chats", resolve.path)
	}
	if resolve.body["property_id"] != "prop-1" || resolve.body["advertiser_id"] != "owner-1" {
		t.Fatalf("body de resolveChat incorrecto: %+v", resolve.body)
	}
	if resolve.header.Get("X-User-Id") != "tenant-1" {
		t.Fatalf("X-User-Id en resolveChat: got %q, want tenant-1", resolve.header.Get("X-User-Id"))
	}

	msg := gw.at(1)
	if msg.path != "/api/v1/chats/chat-1/messages" {
		t.Fatalf("segunda llamada: got %s, want /api/v1/chats/chat-1/messages", msg.path)
	}
	if msg.body["message_type"] != "system" {
		t.Fatalf("message_type: got %v, want system", msg.body["message_type"])
	}
	text, _ := msg.body["body"].(string)
	if text == "" {
		t.Fatal("el body del mensaje de sistema no debería estar vacío")
	}
}

func TestReservationNotificationSubscriber_ReservaCancelada_NotificaSinTenant(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	payload := `{"aggregate_id": "res-1", "property_id": "prop-1"}`
	publishJSON(t, js, "contracts.reservation.cancelled", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return gw.count() >= 2 }, 3*time.Second)

	resolve := gw.at(0)
	if resolve.header.Get("X-User-Id") != "" {
		t.Fatalf("X-User-Id debería ir vacío en un cancel (no hay tenant_id en el payload): got %q", resolve.header.Get("X-User-Id"))
	}
}

func TestReservationNotificationSubscriber_JSONMalformado_NoLlamaAlGateway(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	publishJSON(t, js, "contracts.reservation.confirmed", `{"invalido"`)

	runSubscribeAndCancel(t, sub.StartConsume, nil, 0)

	if gw.count() != 0 {
		t.Fatalf("un JSON malformado no debería haber generado llamadas HTTP, hubo %d", gw.count())
	}
}

func TestReservationNotificationSubscriber_ResolveChatFalla_NoPosteaMensaje(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveStatus = http.StatusInternalServerError
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	payload := `{"aggregate_id": "res-1", "property_id": "prop-1", "tenant_id": "tenant-1", "owner_id": "owner-1"}`
	publishJSON(t, js, "contracts.reservation.confirmed", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return gw.countPath("/api/v1/chats") >= 1 }, 3*time.Second)

	if gw.countPath("/api/v1/chats/chat-1/messages") != 0 {
		t.Fatalf("si resolveChat falla, no debería intentarse postear el mensaje: %d intentos", gw.countPath("/api/v1/chats/chat-1/messages"))
	}
}

func TestReservationNotificationSubscriber_ResolveChatSinID_RetornaError(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveBody = `{}`
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	payload := `{"aggregate_id": "res-1", "property_id": "prop-1", "tenant_id": "tenant-1", "owner_id": "owner-1"}`
	publishJSON(t, js, "contracts.reservation.confirmed", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return gw.countPath("/api/v1/chats") >= 1 }, 3*time.Second)

	if gw.countPath("/api/v1/chats/chat-1/messages") != 0 {
		t.Fatalf("sin id de chat no debería llegar a postear el mensaje: %d intentos", gw.countPath("/api/v1/chats/chat-1/messages"))
	}
}

func TestReservationNotificationSubscriber_PostMensajeFalla_NoRompeElLoop(t *testing.T) {
	gw := newFakeChatGateway()
	gw.messageStatus = http.StatusInternalServerError
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	payload := `{"aggregate_id": "res-1", "property_id": "prop-1", "tenant_id": "tenant-1", "owner_id": "owner-1"}`
	publishJSON(t, js, "contracts.reservation.confirmed", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool {
		return gw.countPath("/api/v1/chats") >= 1 && gw.countPath("/api/v1/chats/chat-1/messages") >= 1
	}, 3*time.Second)
}

func TestReservationNotificationSubscriber_CancelacionDeContexto_TerminaLimpiamente(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	sub := natsadapter.NewReservationNotificationSubscriber(js)

	runSubscribeAndCancel(t, sub.StartConsume, nil, 0)
}
