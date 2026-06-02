package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"inmo.platform/contexts/api-gateway/internal/config"
	"inmo.platform/contexts/api-gateway/internal/proxy"
)

// ─── infraestructura de test ──────────────────────────────────────────────────

const routerTestSecret = "router_test_secret"

// upstreamSpy es un servidor de test que registra si fue llamado
// y con qué request, permitiendo verificar routing y forwarding.
// El mutex protege el acceso concurrente: httptest.NewServer ejecuta
// el handler en su propia goroutine mientras el test lee desde la goroutine principal.
type upstreamSpy struct {
	mu      sync.Mutex
	server  *httptest.Server
	called  bool
	lastReq *http.Request
}

func newUpstreamSpy(status int, body string) *upstreamSpy {
	spy := &upstreamSpy{}
	spy.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spy.mu.Lock()
		spy.called = true
		spy.lastReq = r
		spy.mu.Unlock()
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	return spy
}

func (s *upstreamSpy) URL() string {
	return s.server.URL
}

func (s *upstreamSpy) Close() {
	s.server.Close()
}

func (s *upstreamSpy) wasCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

func (s *upstreamSpy) getLastReq() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastReq
}

func (s *upstreamSpy) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called = false
	s.lastReq = nil
}

// buildConfig construye un *config.Config apuntando todos los upstreams
// a los servidores de test provistos.
func buildConfig(
	authURL, catalogURL, crmURL,
	maintenanceURL, financesURL, contractsURL string,
) *config.Config {
	return &config.Config{
		Port:           ":9999",
		AuthURL:        authURL,
		CatalogURL:     catalogURL,
		CRMURL:         crmURL,
		MaintenanceURL: maintenanceURL,
		FinancesURL:    financesURL,
		ContractsURL:   contractsURL,
		JWTSecret:      routerTestSecret,
	}
}

// makeRouterToken genera un JWT válido para los tests de rutas privadas.
func makeRouterToken(t *testing.T, userID string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(routerTestSecret))
	if err != nil {
		t.Fatalf("makeRouterToken: %v", err)
	}
	return signed
}

// executeRequest dispara un request contra el router y devuelve el recorder.
func executeRequest(t *testing.T, handler http.Handler, method, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ─── setup compartido ─────────────────────────────────────────────────────────

// setupRouter crea todos los spies, construye el config y devuelve el handler
// listo para usar. El caller es responsable de llamar a closeAll().
func setupRouter(t *testing.T) (handler http.Handler, spies map[string]*upstreamSpy, closeAll func()) {
	t.Helper()

	auth := newUpstreamSpy(http.StatusOK, `{"token":"abc"}`)
	catalog := newUpstreamSpy(http.StatusOK, `{"properties":[]}`)
	crm := newUpstreamSpy(http.StatusCreated, `{"id":"lead-1"}`)
	maintenance := newUpstreamSpy(http.StatusOK, `{}`)
	finances := newUpstreamSpy(http.StatusOK, `{}`)
	contracts := newUpstreamSpy(http.StatusOK, `{}`)

	cfg := buildConfig(
		auth.URL(),
		catalog.URL(),
		crm.URL(),
		maintenance.URL(),
		finances.URL(),
		contracts.URL(),
	)

	router := proxy.NewRouter(cfg)
	handler = router.MapRoutes()

	spies = map[string]*upstreamSpy{
		"auth":        auth,
		"catalog":     catalog,
		"crm":         crm,
		"maintenance": maintenance,
		"finances":    finances,
		"contracts":   contracts,
	}

	closeAll = func() {
		for _, s := range spies {
			s.Close()
		}
	}

	return handler, spies, closeAll
}

// ─── rutas públicas ───────────────────────────────────────────────────────────

func TestRouter_PublicRoutes_NoAuthRequired(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	tests := []struct {
		name             string
		method           string
		path             string
		expectedUpstream string
	}{
		{
			name:             "POST /auth/login — ruta pública → auth upstream",
			method:           http.MethodPost,
			path:             "/api/v1/auth/login",
			expectedUpstream: "auth",
		},
		{
			name:             "POST /auth/register — ruta pública → auth upstream",
			method:           http.MethodPost,
			path:             "/api/v1/auth/register",
			expectedUpstream: "auth",
		},
		{
			name:             "GET /auth/verify — ruta pública → auth upstream",
			method:           http.MethodGet,
			path:             "/api/v1/auth/verify",
			expectedUpstream: "auth",
		},
		{
			name:             "POST /auth/sso/google — ruta pública → auth upstream",
			method:           http.MethodPost,
			path:             "/api/v1/auth/sso/google",
			expectedUpstream: "auth",
		},
		{
			name:             "POST /auth/sso/meta — ruta pública → auth upstream",
			method:           http.MethodPost,
			path:             "/api/v1/auth/sso/meta",
			expectedUpstream: "auth",
		},
		{
			name:             "GET /properties — lectura pública → catalog upstream",
			method:           http.MethodGet,
			path:             "/api/v1/properties",
			expectedUpstream: "catalog",
		},
		{
			name:             "GET /properties/{id} — lectura pública → catalog upstream",
			method:           http.MethodGet,
			path:             "/api/v1/properties/PROP-001",
			expectedUpstream: "catalog",
		},
		{
			name:             "GET /properties/{id}/media — fotos públicas → catalog upstream",
			method:           http.MethodGet,
			path:             "/api/v1/properties/PROP-001/media",
			expectedUpstream: "catalog",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset todos los spies antes de cada caso
			for _, s := range spies {
				s.reset()
			}

			// Sin token — si el router requiriera auth, respondería 401
			rr := executeRequest(t, handler, tc.method, tc.path, "")

			// No debe retornar 401
			if rr.Code == http.StatusUnauthorized {
				t.Errorf("%s %s: got 401, la ruta debería ser pública", tc.method, tc.path)
			}

			// El upstream esperado debe haber sido llamado
			if !spies[tc.expectedUpstream].wasCalled() {
				t.Errorf("%s %s: upstream %q no fue llamado", tc.method, tc.path, tc.expectedUpstream)
			}

			// Ningún otro upstream debe haber sido llamado
			for name, spy := range spies {
				if name != tc.expectedUpstream && spy.wasCalled() {
					t.Errorf("%s %s: upstream %q fue llamado inesperadamente", tc.method, tc.path, name)
				}
			}
		})
	}
}

// ─── rutas privadas sin token → 401 ──────────────────────────────────────────

func TestRouter_PrivateRoutes_Unauthenticated_Returns401(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST /properties sin token", http.MethodPost, "/api/v1/properties"},
		{"PUT /properties/{id} sin token", http.MethodPut, "/api/v1/properties/PROP-001"},
		{"POST /catalog/profiles sin token", http.MethodPost, "/api/v1/catalog/profiles"},
		{"GET /leads sin token", http.MethodGet, "/api/v1/leads"},
		{"POST /leads sin token", http.MethodPost, "/api/v1/leads"},
		// tickets, finances y contracts están registrados con trailing slash en el mux principal
		// (/api/v1/tickets/). Usamos rutas con sub-path para evitar el 301 automático del mux.
		{"GET /tickets/{id} sin token", http.MethodGet, "/api/v1/tickets/ticket-001"},
		{"POST /tickets/{id}/close sin token", http.MethodPost, "/api/v1/tickets/ticket-001/close"},
		{"GET /finances/settlements sin token", http.MethodGet, "/api/v1/finances/settlements"},
		{"GET /contracts/{id} sin token", http.MethodGet, "/api/v1/contracts/ctr-001"},
		{"POST /reservations sin token", http.MethodPost, "/api/v1/reservations"},
		{"GET /reservations/{id} sin token", http.MethodGet, "/api/v1/reservations/res-123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range spies {
				s.reset()
			}

			rr := executeRequest(t, handler, tc.method, tc.path, "")

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s %s sin token: got %d, want 401", tc.method, tc.path, rr.Code)
			}

			// Verificar que ningún upstream fue contactado
			for name, spy := range spies {
				if spy.wasCalled() {
					t.Errorf("%s %s: upstream %q no debería haberse llamado sin token", tc.method, tc.path, name)
				}
			}
		})
	}
}

// ─── rutas privadas con token válido → upstream correcto ─────────────────────

func TestRouter_PrivateRoutes_Authenticated_ReachCorrectUpstream(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	token := makeRouterToken(t, "user-test-123")
	bearer := "Bearer " + token

	tests := []struct {
		name             string
		method           string
		path             string
		expectedUpstream string
	}{
		{
			name:             "POST /properties con token → catalog",
			method:           http.MethodPost,
			path:             "/api/v1/properties",
			expectedUpstream: "catalog",
		},
		{
			name:             "POST /catalog/profiles con token → catalog",
			method:           http.MethodPost,
			path:             "/api/v1/catalog/profiles",
			expectedUpstream: "catalog",
		},
		{
			name:             "GET /leads con token → crm",
			method:           http.MethodGet,
			path:             "/api/v1/leads",
			expectedUpstream: "crm",
		},
		{
			name:             "POST /leads con token → crm",
			method:           http.MethodPost,
			path:             "/api/v1/leads",
			expectedUpstream: "crm",
		},
		{
			name:             "POST /tickets/{id}/assign con token → maintenance",
			method:           http.MethodPost,
			path:             "/api/v1/tickets/ticket-abc/assign",
			expectedUpstream: "maintenance",
		},
		{
			name:             "GET /tickets/{id} con token → maintenance",
			method:           http.MethodGet,
			path:             "/api/v1/tickets/ticket-abc",
			expectedUpstream: "maintenance",
		},
		{
			name:             "GET /finances/settlements con token → finances",
			method:           http.MethodGet,
			path:             "/api/v1/finances/settlements",
			expectedUpstream: "finances",
		},
		{
			name:             "POST /finances/settlements con token → finances",
			method:           http.MethodPost,
			path:             "/api/v1/finances/settlements",
			expectedUpstream: "finances",
		},
		{
			name:             "POST /contracts/{id}/activate con token → contracts",
			method:           http.MethodPost,
			path:             "/api/v1/contracts/ctr-001/activate",
			expectedUpstream: "contracts",
		},
		{
			name:             "GET /contracts/{id} con token → contracts",
			method:           http.MethodGet,
			path:             "/api/v1/contracts/ctr-001",
			expectedUpstream: "contracts",
		},
		{
			name:             "POST /reservations con token → contracts",
			method:           http.MethodPost,
			path:             "/api/v1/reservations",
			expectedUpstream: "contracts",
		},
		{
			name:             "GET /reservations/{id} con token → contracts",
			method:           http.MethodGet,
			path:             "/api/v1/reservations/res-001",
			expectedUpstream: "contracts",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range spies {
				s.reset()
			}

			rr := executeRequest(t, handler, tc.method, tc.path, bearer)

			// No debe ser 401 ni 403
			if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
				t.Errorf("%s %s con token válido: got %d, want upstream response", tc.method, tc.path, rr.Code)
				return
			}

			// El upstream correcto debe haber sido llamado
			if !spies[tc.expectedUpstream].wasCalled() {
				t.Errorf("%s %s: upstream %q no fue llamado con token válido", tc.method, tc.path, tc.expectedUpstream)
			}

			// Ningún otro upstream incorrecto debe haberse llamado
			for name, spy := range spies {
				if name != tc.expectedUpstream && spy.wasCalled() {
					t.Errorf("%s %s: upstream %q fue llamado inesperadamente", tc.method, tc.path, name)
				}
			}
		})
	}
}

// ─── host rewriting ───────────────────────────────────────────────────────────

// TestRouter_HostRewriting verifica que el Director del reverse proxy setea
// req.Host al host del upstream target, no el host original del cliente.
// Sin este rewrite algunos upstreams (nginx, caddy) rechazan el request.
func TestRouter_HostRewriting(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	token := "Bearer " + makeRouterToken(t, "user-host-test")

	tests := []struct {
		name    string
		method  string
		path    string
		spyName string
		bearer  string
	}{
		{
			name:    "auth upstream — login",
			method:  http.MethodPost,
			path:    "/api/v1/auth/login",
			spyName: "auth",
			bearer:  "",
		},
		{
			name:    "catalog upstream — GET properties",
			method:  http.MethodGet,
			path:    "/api/v1/properties",
			spyName: "catalog",
			bearer:  "",
		},
		{
			name:    "crm upstream — GET leads",
			method:  http.MethodGet,
			path:    "/api/v1/leads",
			spyName: "crm",
			bearer:  token,
		},
		{
			name:    "contracts upstream — POST reservations",
			method:  http.MethodPost,
			path:    "/api/v1/reservations",
			spyName: "contracts",
			bearer:  token,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, s := range spies {
				s.reset()
			}

			rr := executeRequest(t, handler, tc.method, tc.path, tc.bearer)

			if rr.Code == http.StatusUnauthorized {
				t.Fatalf("%s %s: got 401 inesperado", tc.method, tc.path)
			}

			spy := spies[tc.spyName]
			if !spy.wasCalled() {
				t.Fatalf("upstream %q no fue llamado", tc.spyName)
			}

			// El Host que recibe el upstream debe ser el host del servidor spy,
			// no "example.com" (el default de httptest.NewRequest).
			expectedHost := strings.TrimPrefix(spy.URL(), "http://")
			gotHost := spy.getLastReq().Host
			if gotHost != expectedHost {
				t.Errorf("Host rewriting: upstream recibió Host=%q, want %q", gotHost, expectedHost)
			}
		})
	}
}

// ─── X-User-Id forwarding ─────────────────────────────────────────────────────

// TestRouter_XUserIdForwarded verifica que el header X-User-Id extraído del JWT
// llega al upstream en rutas privadas. Esto es crítico: los servicios downstream
// confían en este header para la autorización a nivel de recurso.
func TestRouter_XUserIdForwarded(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	const userID = "user-forward-test-999"
	token := "Bearer " + makeRouterToken(t, userID)

	spies["catalog"].reset()

	rr := executeRequest(t, handler, http.MethodPost, "/api/v1/properties", token)

	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("POST /properties con token válido: got 401")
	}

	if !spies["catalog"].wasCalled() {
		t.Fatal("upstream catalog no fue llamado")
	}

	gotUserID := spies["catalog"].getLastReq().Header.Get("X-User-Id")
	if gotUserID != userID {
		t.Errorf("X-User-Id forwarded: got %q, want %q", gotUserID, userID)
	}
}

// ─── token expirado en rutas privadas ────────────────────────────────────────

// TestRouter_ExpiredToken_Returns401 verifica que un token expirado
// es rechazado por el middleware antes de llegar al upstream.
func TestRouter_ExpiredToken_Returns401(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	expiredClaims := jwt.MapClaims{
		"sub": "user-expired",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	signed, err := expiredToken.SignedString([]byte(routerTestSecret))
	if err != nil {
		t.Fatalf("no se pudo generar token expirado: %v", err)
	}

	privatePaths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/properties"},
		{http.MethodGet, "/api/v1/leads"},
		{http.MethodPost, "/api/v1/reservations"},
	}

	for _, p := range privatePaths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			for _, s := range spies {
				s.reset()
			}

			rr := executeRequest(t, handler, p.method, p.path, "Bearer "+signed)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("token expirado en %s %s: got %d, want 401", p.method, p.path, rr.Code)
			}

			for name, spy := range spies {
				if spy.wasCalled() {
					t.Errorf("upstream %q no debe llamarse con token expirado", name)
				}
			}
		})
	}
}

// ─── CORS en rutas del router ─────────────────────────────────────────────────

// TestRouter_OPTIONS_HandledByCORSLayer verifica que el router puro (sin CORSHandler)
// no requiere autenticación para rutas de auth públicas.
// OPTIONS es manejado por CORSHandler en main.go — ese comportamiento
// está cubierto en cors_test.go. Acá verificamos que las rutas públicas
// nunca retornan 401 independientemente del método.
func TestRouter_PublicAuthRoutes_NeverReturn401(t *testing.T) {
	handler, _, closeAll := setupRouter(t)
	defer closeAll()

	publicPaths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodGet, "/api/v1/auth/verify"},
	}

	for _, p := range publicPaths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			rr := executeRequest(t, handler, p.method, p.path, "")
			if rr.Code == http.StatusUnauthorized {
				t.Errorf("%s %s pública: got 401, nunca debe requerir auth", p.method, p.path)
			}
		})
	}
}

// ─── aislamiento de upstream — un fallo no afecta a otros ────────────────────

// TestRouter_UpstreamIsolation verifica que cuando se llama a una ruta,
// exactamente UN upstream es contactado y ningún otro.
// Garantiza que el routing no tiene efectos secundarios entre bounded contexts.
func TestRouter_UpstreamIsolation(t *testing.T) {
	handler, spies, closeAll := setupRouter(t)
	defer closeAll()

	token := "Bearer " + makeRouterToken(t, "user-isolation")

	// POST /api/v1/leads debe ir SOLO a CRM
	for _, s := range spies {
		s.reset()
	}

	executeRequest(t, handler, http.MethodPost, "/api/v1/leads", token)

	calledUpstreams := []string{}
	for name, spy := range spies {
		if spy.wasCalled() {
			calledUpstreams = append(calledUpstreams, name)
		}
	}

	if len(calledUpstreams) != 1 {
		t.Errorf("POST /leads: se llamaron %d upstreams %v, want exactamente 1 (crm)",
			len(calledUpstreams), calledUpstreams)
	}
	if !spies["crm"].wasCalled() {
		t.Errorf("POST /leads: upstream crm no fue llamado")
	}
}
