package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inmo.platform/contexts/auth-identity/internal/adapters/httpapi"
)

// ─── NewRouter ──────────────────────────────────────────────────────────────
//
// Estos tests no vuelven a ejercitar la lógica de negocio (eso ya lo cubren
// handlers_test.go y los tests de application) — solo confirman que cada ruta
// del mux está atada al método HTTP y al handler correctos. Un typo en el
// path o en el verbo (ej: GET en vez de POST) pasaría desapercibido para el
// resto de los tests, que invocan los handlers directamente sin pasar por el mux.

func TestNewRouter_RutasResuelvenAlHandlerCorrecto(t *testing.T) {
	router := httpapi.NewRouter(newTestHandler(newDefaultEnv()))

	cases := []struct {
		name             string
		method           string
		path             string
		body             string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name: "register", method: http.MethodPost, path: "/api/v1/auth/register",
			body: "", wantStatus: http.StatusBadRequest, wantBodyContains: "Payload JSON inválido",
		},
		{
			name: "login", method: http.MethodPost, path: "/api/v1/auth/login",
			body: "", wantStatus: http.StatusBadRequest, wantBodyContains: "Payload JSON inválido",
		},
		{
			name: "verify email", method: http.MethodGet, path: "/api/v1/auth/verify",
			wantStatus: http.StatusBadRequest, wantBodyContains: "token' es obligatorio",
		},
		{
			name: "sso config", method: http.MethodGet, path: "/api/v1/auth/sso/config",
			wantStatus: http.StatusOK, wantBodyContains: "google_client_id",
		},
		{
			name: "google login", method: http.MethodPost, path: "/api/v1/auth/sso/google",
			body: "", wantStatus: http.StatusBadRequest, wantBodyContains: "Payload JSON inválido",
		},
		{
			name: "meta login", method: http.MethodPost, path: "/api/v1/auth/sso/meta",
			body: "", wantStatus: http.StatusBadRequest, wantBodyContains: "Payload JSON inválido",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBodyContains) {
				t.Errorf("body: got %q, want que contenga %q", rec.Body.String(), tc.wantBodyContains)
			}
		})
	}
}

func TestNewRouter_MetodoNoPermitido_Retorna405(t *testing.T) {
	router := httpapi.NewRouter(newTestHandler(newDefaultEnv()))

	// El mux registra /register solo para POST — GET sobre la misma ruta debe
	// rechazarse a nivel de enrutamiento, sin siquiera llegar al handler.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/register", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNewRouter_RutaInexistente_Retorna404(t *testing.T) {
	router := httpapi.NewRouter(newTestHandler(newDefaultEnv()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/no-existe", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}
