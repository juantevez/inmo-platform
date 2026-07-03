package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/shared/pkg/health"
)

func httptestBody(body string) io.Reader {
	if body == "" {
		return nil
	}
	return strings.NewReader(body)
}

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// NewRouter exige *ContractHandler / *ReservationHandler reales (no son
// interfaces), así que se construyen con casos de uso reales sobre fakes con
// fixtures no configuradas — a estas rutas solo les interesa confirmar el
// dispatch del mux, no el resultado de negocio (eso ya se cubre en
// handler_test.go / reservation_handler_test.go).

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	db, _ := newMockDB(t)
	contractRepo := &fakeContractRepo{}
	resRepo := &fakeReservationRepo{}
	snapRepo := &fakeSnapshotRepo{}

	h := httpapi.NewContractHandler(
		application.NewCreateContractUseCase(contractRepo),
		application.NewActivateContractUseCase(db, contractRepo),
	)
	rh := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, resRepo, snapRepo),
		application.NewConfirmReservationUseCase(db, resRepo, snapRepo),
		application.NewCancelReservationUseCase(db, resRepo),
		application.NewGetReservationUseCase(resRepo, snapRepo),
		application.NewGetOwnerReservationsUseCase(resRepo, snapRepo),
	)
	checker := health.NewChecker(nil, nil)

	return httpapi.NewRouter(h, rh, checker)
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
		{"crear contrato sin body válido", http.MethodPost, "/api/v1/contracts", "{invalido"},
		{"activar contrato sin id", http.MethodPost, "/api/v1/contracts/activate", ""},
		{"listar reservas del owner sin auth", http.MethodGet, "/api/v1/reservations/owner", ""},
		{"crear reserva sin auth", http.MethodPost, "/api/v1/reservations", "{}"},
		{"ver reserva sin fixture", http.MethodGet, "/api/v1/reservations/res-1", ""},
		{"confirmar reserva sin auth", http.MethodPost, "/api/v1/reservations/res-1/confirm", ""},
		{"cancelar reserva sin auth", http.MethodPost, "/api/v1/reservations/res-1/cancel", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, httptestBody(tc.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Todas estas rutas fallan por falta de auth/fixtures/body — lo
			// importante es que NINGUNA devuelva 404, confirmando que el mux
			// las enrutó al handler real.
			if rec.Code == http.StatusNotFound {
				t.Fatalf("la ruta %s %s no debería devolver 404 (no debería estar sin registrar)", tc.method, tc.path)
			}
		})
	}
}

func TestRouter_ListarOwner_AntesDeReservationsID_NoColisiona(t *testing.T) {
	// GET /api/v1/reservations/owner debe resolver a HandleListOwner, no a
	// HandleGet con id="owner" — de lo contrario el panel del propietario
	// quedaría inalcanzable.
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/owner", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// HandleListOwner responde 400 por falta de X-User-Id; HandleGet en
	// cambio intentaría buscar una reserva "owner" y respondería 404/500.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d (esperaba caer en HandleListOwner)", rec.Code, http.StatusBadRequest)
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
	// /api/v1/contracts está registrado solo para POST — GET no existe.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/contracts", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
