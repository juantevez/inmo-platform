package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"inmo.platform/contexts/api-gateway/internal/middleware"
)

// ─── valores esperados de los headers CORS ───────────────────────────────────

// expectedCORSHeaders centraliza los valores que setCORSHeaders debe inyectar.
// Si el código productivo cambia algún valor, el test falla en exactamente un lugar.
var expectedCORSHeaders = map[string]string{
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Allow-Methods":  "GET, POST, PUT, DELETE, PATCH, OPTIONS",
	"Access-Control-Allow-Headers":  "Content-Type, Authorization, X-Requested-With, X-User-Id, X-User-Role",
	"Access-Control-Expose-Headers": "Content-Length",
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// assertCORSHeaders verifica que todos los headers CORS esperados están presentes
// en la respuesta con sus valores correctos.
func assertCORSHeaders(t *testing.T, header http.Header) {
	t.Helper()
	for key, want := range expectedCORSHeaders {
		got := header.Get(key)
		if got != want {
			t.Errorf("header %q: got %q, want %q", key, got, want)
		}
	}
}

// upstreamWith construye un handler que setea los headers y status dados
// antes de escribir el body. Simula un servicio upstream real.
func upstreamWith(status int, headers map[string]string, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		if body != "" {
			_, _ = io.WriteString(w, body)
		}
	})
}

// ─── CORSHandler — preflight OPTIONS ─────────────────────────────────────────

// TestCORSHandler_Options_Returns204 verifica que las peticiones OPTIONS
// reciben 204 con todos los headers CORS correctos.
func TestCORSHandler_Options_Returns204(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("el upstream NO debe ser llamado en una petición OPTIONS")
	})

	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/properties", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS: status got %d, want 204", rr.Code)
	}

	assertCORSHeaders(t, rr.Header())
}

// TestCORSHandler_Options_EmptyBody verifica que OPTIONS no devuelve body.
func TestCORSHandler_Options_EmptyBody(t *testing.T) {
	handler := middleware.CORSHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if body := rr.Body.String(); body != "" {
		t.Errorf("OPTIONS body debería estar vacío, got %q", body)
	}
}

// TestCORSHandler_Options_NextNotCalled verifica que OPTIONS hace short-circuit
// y el upstream nunca se ejecuta.
func TestCORSHandler_Options_NextNotCalled(t *testing.T) {
	nextCalled := false
	spy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.CORSHandler(spy)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/any", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if nextCalled {
		t.Error("el upstream no debe ejecutarse en una petición OPTIONS")
	}
}

// ─── CORSHandler — requests no-OPTIONS ───────────────────────────────────────

// TestCORSHandler_Get_InjectsCORSHeaders verifica que una petición GET normal
// recibe los headers CORS correctos aunque el upstream no los devuelva.
func TestCORSHandler_Get_InjectsCORSHeaders(t *testing.T) {
	upstream := upstreamWith(http.StatusOK, nil, "")
	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET: status got %d, want 200", rr.Code)
	}
	assertCORSHeaders(t, rr.Header())
}

// TestCORSHandler_Post_InjectsCORSHeaders verifica el mismo comportamiento en POST.
func TestCORSHandler_Post_InjectsCORSHeaders(t *testing.T) {
	upstream := upstreamWith(http.StatusCreated, nil, `{"id":"123"}`)
	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("POST: status got %d, want 201", rr.Code)
	}
	assertCORSHeaders(t, rr.Header())
}

// TestCORSHandler_UpstreamCORSHeadersAreRemoved verifica el caso crítico:
// el upstream devuelve sus propios headers CORS y el gateway debe eliminarlos
// antes de poner los suyos. Esto previene headers duplicados o contradictorios.
func TestCORSHandler_UpstreamCORSHeadersAreRemoved(t *testing.T) {
	// Upstream que devuelve headers CORS propios (como haría un servicio mal configurado)
	upstreamHeaders := map[string]string{
		"Access-Control-Allow-Origin":      "https://malicious.example.com",
		"Access-Control-Allow-Methods":     "DELETE",
		"Access-Control-Allow-Headers":     "X-Custom-Header",
		"Access-Control-Allow-Credentials": "true",
		"Access-Control-Expose-Headers":    "X-Internal-Id",
	}
	upstream := upstreamWith(http.StatusOK, upstreamHeaders, "")
	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Los headers del upstream deben haber sido eliminados y reemplazados
	// por los del gateway. Verificamos que los valores son los correctos
	// (no los del upstream).
	assertCORSHeaders(t, rr.Header())

	// Verificación extra: Access-Control-Allow-Credentials del upstream
	// no debe aparecer en la respuesta (el gateway no lo setea).
	// Si quedara "true" del upstream, sería un bug de seguridad.
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got == "true" {
		t.Errorf("Access-Control-Allow-Credentials del upstream no fue eliminado: got %q", got)
	}
}

// TestCORSHandler_NoDuplicateHeaders verifica que cada header CORS aparece
// exactamente una vez en la respuesta, incluso cuando el upstream también los envía.
// net/http permite múltiples valores por header — este test evita esa regresión.
func TestCORSHandler_NoDuplicateHeaders(t *testing.T) {
	// Upstream que intenta setear el mismo header CORS que el gateway
	upstreamHeaders := map[string]string{
		"Access-Control-Allow-Origin": "https://upstream.example.com",
	}
	upstream := upstreamWith(http.StatusOK, upstreamHeaders, "")
	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Header().Values() devuelve todos los valores del header (slice).
	// Si hay duplicados, el slice tendrá más de un elemento.
	values := rr.Header().Values("Access-Control-Allow-Origin")
	if len(values) != 1 {
		t.Errorf("Access-Control-Allow-Origin: got %d valores %v, want exactamente 1", len(values), values)
	}
	if values[0] != "*" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want %q", values[0], "*")
	}
}

// TestCORSHandler_UpstreamBodyPreserved verifica que el body del upstream
// llega intacto al cliente después de pasar por corsWriter.
func TestCORSHandler_UpstreamBodyPreserved(t *testing.T) {
	const expectedBody = `{"properties":[],"total":0}`
	upstream := upstreamWith(http.StatusOK, nil, expectedBody)
	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Body.String(); got != expectedBody {
		t.Errorf("body: got %q, want %q", got, expectedBody)
	}
}

// TestCORSHandler_UpstreamStatusPreserved verifica que el status code del upstream
// se preserva a través de corsWriter (no se fija siempre en 200).
func TestCORSHandler_UpstreamStatusPreserved(t *testing.T) {
	tests := []struct {
		name           string
		upstreamStatus int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := upstreamWith(tc.upstreamStatus, nil, "")
			handler := middleware.CORSHandler(upstream)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.upstreamStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.upstreamStatus)
			}
		})
	}
}

// ─── corsWriter — comportamiento interno ──────────────────────────────────────

// TestCorsWriter_WriteWithoutExplicitWriteHeader verifica que corsWriter.Write
// llama a WriteHeader(200) implícitamente si no fue llamado antes.
// Esto replica el comportamiento estándar de http.ResponseWriter.
func TestCorsWriter_WriteWithoutExplicitWriteHeader(t *testing.T) {
	// Upstream que escribe body sin llamar a WriteHeader explícitamente
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello")
		// Nota: NO se llama w.WriteHeader() — Go lo hace implícito en Write()
	})

	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// El status implícito debe ser 200
	if rr.Code != http.StatusOK {
		t.Errorf("Write sin WriteHeader previo: status got %d, want 200", rr.Code)
	}

	// Los headers CORS deben haberse inyectado igual
	assertCORSHeaders(t, rr.Header())

	// El body debe haberse escrito
	if got := rr.Body.String(); got != "hello" {
		t.Errorf("body: got %q, want %q", got, "hello")
	}
}

// TestCorsWriter_WriteHeaderIdempotent verifica que llamar a WriteHeader dos veces
// no causa una doble escritura del status code (wroteHeader protege contra esto).
// En net/http nativo, el segundo WriteHeader es ignorado con un warning en el log.
// corsWriter debe hacer lo mismo de forma limpia.
func TestCorsWriter_WriteHeaderIdempotent(t *testing.T) {
	// Upstream que llama WriteHeader dos veces (upstream mal programado)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.WriteHeader(http.StatusInternalServerError) // segunda llamada — debe ignorarse
	})

	handler := middleware.CORSHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// El status debe ser el del primer WriteHeader
	if rr.Code != http.StatusOK {
		t.Errorf("WriteHeader idempotente: got %d, want 200 (primer status)", rr.Code)
	}

	// Los headers CORS solo deben aparecer una vez (sin duplicados por doble WriteHeader)
	values := rr.Header().Values("Access-Control-Allow-Origin")
	if len(values) != 1 {
		t.Errorf("Access-Control-Allow-Origin duplicado por doble WriteHeader: got %v", values)
	}
}

// ─── Métodos HTTP varios ──────────────────────────────────────────────────────

// TestCORSHandler_AllMethods verifica que todos los métodos HTTP (excepto OPTIONS)
// pasan al upstream y reciben headers CORS en la respuesta.
func TestCORSHandler_AllMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			upstream := upstreamWith(http.StatusOK, nil, "")
			handler := middleware.CORSHandler(upstream)

			req := httptest.NewRequest(method, "/api/v1/resource", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: status got %d, want 200", method, rr.Code)
			}
			assertCORSHeaders(t, rr.Header())
		})
	}
}
