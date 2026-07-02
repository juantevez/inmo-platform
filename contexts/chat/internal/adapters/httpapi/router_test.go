package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/adapters/httpapi"
	"inmo.platform/contexts/chat/internal/adapters/websocket"
	"inmo.platform/shared/pkg/health"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// NewRouter exige un *websocket.Hub y un *health.Checker reales (no son
// interfaces). websocket.NewHub() no tiene dependencias externas, así que se
// usa tal cual. health.Checker sólo se ejercita vía LiveHandler acá — esa
// ruta no toca ni la DB ni NATS, así que alcanza con pasar nil en ambos (ver
// el mismo razonamiento aplicado en el router de catalog).

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	h := newTestHandler(newDefaultEnv())
	hub := websocket.NewHub()
	checker := health.NewChecker(nil, nil)

	return httpapi.NewRouter(h, hub, checker)
}

// ─── Health ─────────────────────────────────────────────────────────────────

func TestRouter_HealthLive_Retorna200(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── Rutas REST resuelven al handler correcto ──────────────────────────────

func TestRouter_RutasResuelvenAlHandlerCorrecto(t *testing.T) {
	router := newRouter(t)

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"crear conversación sin body válido", http.MethodPost, "/api/v1/chats", "{}", http.StatusBadRequest},
		{"listar conversaciones sin auth", http.MethodGet, "/api/v1/chats", "", http.StatusBadRequest},
		{"ver mensajes sin auth", http.MethodGet, "/api/v1/chats/conv-1", "", http.StatusBadRequest},
		{"enviar mensaje sin auth", http.MethodPost, "/api/v1/chats/conv-1/messages", "{}", http.StatusBadRequest},
		{"proponer visita sin auth", http.MethodPost, "/api/v1/chats/conv-1/visit-proposals", "{}", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Todas fallan por falta de X-User-Id (400) — lo importante es que
			// NINGUNA devuelva 404, confirmando que el mux las enrutó al handler real.
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestRouter_WebSocketRoute_NoEs404(t *testing.T) {
	// golang.org/x/net/websocket intenta hacer Hijack() de la conexión ni bien
	// entra al handler — httptest.NewRecorder no implementa http.Hijacker y
	// panickea. Hace falta un listener real (httptest.NewServer) para que la
	// conexión subyacente soporte el hijack, aunque el handshake en sí no se
	// complete (no mandamos los headers de Upgrade).
	router := newRouter(t)
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/ws/chats/conv-1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-User-Id", "user-1")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("la ruta no debería ni siquiera responder con un error de transporte: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("status: got 404, want que la ruta /ws/chats/{id} esté registrada")
	}
}

// ─── Rutas / métodos no registrados ─────────────────────────────────────────

func TestRouter_RutaInexistente_Retorna404(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/no-existe", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRouter_MetodoNoPermitido_Retorna405(t *testing.T) {
	router := newRouter(t)
	// /api/v1/chats está registrado para GET y POST — DELETE no existe.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/chats", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
