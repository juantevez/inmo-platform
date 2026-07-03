package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inmo.platform/contexts/crm/internal/adapters/httpapi"
	"inmo.platform/shared/pkg/health"
)

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	h := newHandler(&fakeLeadRepo{})
	checker := health.NewChecker(nil, nil)
	return httpapi.NewRouter(h, checker)
}

func TestRouter_HealthLive_Retorna200(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRouter_RutasResuelvenAlHandlerCorrecto(t *testing.T) {
	router := newRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"get lead sin fixture", http.MethodGet, "/api/v1/leads/lead-1", ""},
		{"contact sin fixture", http.MethodPost, "/api/v1/leads/lead-1/contact", ""},
		{"schedule-visit sin body", http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", ""},
		{"close sin fixture", http.MethodPost, "/api/v1/leads/lead-1/close", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Todas fallan por falta de fixture/body — lo importante es que
			// NINGUNA devuelva 404, confirmando que el mux las enrutó al handler real.
			if rec.Code == http.StatusNotFound {
				t.Fatalf("la ruta %s %s no debería devolver 404", tc.method, tc.path)
			}
		})
	}
}

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
	// /api/v1/leads/{id} está registrado solo para GET — DELETE no existe.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/leads/lead-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
