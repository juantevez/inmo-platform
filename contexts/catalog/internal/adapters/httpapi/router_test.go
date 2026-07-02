package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/health"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// NewRouter exige un *health.Checker real. LiveHandler no toca ni la DB ni NATS,
// así que alcanza con un *sql.DB de sqlmock (Ping responde OK por defecto) y un
// *nats.Conn nil — nunca se ejercita porque estos tests no pegan a /healthz/ready
// (esa lógica es responsabilidad de los tests del paquete health, no del router).

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	checker := health.NewChecker(db, nil)

	propertyHandler, _ := newHandler(t, &fakeRepo{properties: map[string]*domain.Property{}}, &fakeBlockedDatesRepo{})
	mediaHandler := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	profileHandler := newProfileHandler(&fakeProfileRepo{})

	return httpapi.NewRouter(propertyHandler, profileHandler, mediaHandler, checker)
}

func decodeErrBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodeErrBody: %v, body=%q", err, rec.Body.String())
	}
	return got["error"]
}

// ─── Health ─────────────────────────────────────────────────────────────────

func TestRouter_HealthzLive_Retorna200(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── Rutas públicas: no exigen ni auth ni permisos ─────────────────────────

func TestRouter_RutasPublicas_NoExigenAutenticacion(t *testing.T) {
	router := newRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"listar propiedades", http.MethodGet, "/api/v1/properties", ""},
		{"listar media de una propiedad", http.MethodGet, "/api/v1/properties/prop-1/media", ""},
		{"cotizar una propiedad", http.MethodPost, "/api/v1/properties/prop-1/quote", `{"check_in_date":"2025-01-01","check_out_date":"2025-01-05"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Sin X-User-Id ni X-Permissions, igual debe llegar al handler real
			// (que puede devolver 404 por datos inexistentes, pero NUNCA 401/403).
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("status: got %d, want que NO sea 401/403 (ruta pública), body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// ─── Rutas que exigen autenticación simple (requireAuth) ───────────────────

func TestRouter_RutasConRequireAuth_SinXUserId_Retorna401(t *testing.T) {
	router := newRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"reservar propiedad", http.MethodPost, "/api/v1/properties/prop-1/reserve"},
		{"obtener perfil propio", http.MethodGet, "/api/v1/catalog/profiles/me"},
		{"crear perfil", http.MethodPost, "/api/v1/catalog/profiles"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString("{}"))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if got := decodeErrBody(t, rec); got != "Autenticación requerida" {
				t.Errorf("error: got %q, want %q", got, "Autenticación requerida")
			}
		})
	}
}

func TestRouter_ReservarConXUserId_PasaAlHandler(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/reserve", nil)
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// La propiedad no existe → el handler debe llegar a devolver 404, no 401.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ─── Rutas que exigen un permiso específico (requirePermission) ───────────

func TestRouter_RutasConRequirePermission_SinHeader_Retorna403(t *testing.T) {
	router := newRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"publicar propiedad", http.MethodPost, "/api/v1/properties"},
		{"actualizar propiedad", http.MethodPut, "/api/v1/properties/prop-1"},
		{"generar upload url", http.MethodPost, "/api/v1/properties/prop-1/media/upload-url"},
		{"agregar media", http.MethodPost, "/api/v1/properties/prop-1/media"},
		{"borrar media", http.MethodDelete, "/api/v1/properties/prop-1/media/media-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString("{}"))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			if got := decodeErrBody(t, rec); got != "Sin permisos asignados — autenticación requerida" {
				t.Errorf("error: got %q, want el mensaje de falta de permisos", got)
			}
		})
	}
}

func TestRouter_PublicarPropiedad_ConPermisoIncorrecto_Retorna403(t *testing.T) {
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBufferString("{}"))
	req.Header.Set("X-Permissions", "property:delete") // no es property:create
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := decodeErrBody(t, rec); got != "No tenés permiso para realizar esta acción (property:create)" {
		t.Errorf("error: got %q", got)
	}
}

func TestRouter_PublicarPropiedad_ConPermisoCorrecto_PasaAlHandler(t *testing.T) {
	router := newRouter(t)
	// Sin X-User-Id: el permiso alcanza para pasar el middleware, pero el handler
	// de Publish exige su propio X-User-Id — así confirmamos que llegó más allá del router.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBufferString("{}"))
	req.Header.Set("X-Permissions", "property:create")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("status: got 403, no debería bloquear con el permiso correcto, body=%s", rec.Body.String())
	}
	if got := decodeErrBody(t, rec); got == "Sin permisos asignados — autenticación requerida" {
		t.Errorf("el request no debería haber sido frenado por el middleware de permisos")
	}
}

func TestRouter_PermisosConMultiplesValoresYEspacios_Matchea(t *testing.T) {
	// X-Permissions puede traer varios permisos separados por coma — el router
	// debe hacer trim de espacios al comparar cada uno.
	router := newRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBufferString("{}"))
	req.Header.Set("X-Permissions", "property:read, property:create , property:delete")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("status: got 403, want que matchee property:create pese a los espacios, body=%s", rec.Body.String())
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
	// /api/v1/properties está registrado para GET (público) y POST (protegido) — DELETE no existe.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/properties", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ─── Sanity check del *sql.DB usado por el health checker ──────────────────

func TestNewRouter_HealthCheckerDB_PingFunciona(t *testing.T) {
	// Confirma que el *sql.DB de sqlmock realmente soporta Ping antes de confiar
	// en él para health checks — si esto rompiera, LiveHandler no fallaría (no
	// usa la DB), pero ReadyHandler sí, silenciosamente, en otro test.
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: error inesperado: %v", err)
	}
	var _ *sql.DB = db
}
