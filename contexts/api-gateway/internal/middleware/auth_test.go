package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"inmo.platform/contexts/api-gateway/internal/middleware"
)

// ─── constantes de test ──────────────────────────────────────────────────────

const (
	testSecret      = "test_secret_key"
	differentSecret = "another_secret_key"
)

// ─── helpers de generación de tokens ─────────────────────────────────────────

// makeToken genera un JWT firmado con los claims y parámetros dados.
// Centraliza la creación de tokens para todos los tests de este paquete.
func makeToken(t *testing.T, claims jwt.MapClaims, secret string, method jwt.SigningMethod) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeToken: no se pudo firmar el JWT: %v", err)
	}
	return signed
}

// validClaims devuelve claims mínimos válidos: sub presente y exp en el futuro.
func validClaims(userID string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

// expiredClaims devuelve claims con exp en el pasado.
func expiredClaims(userID string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}
}

// nextHandler es el handler de destino del middleware.
// Escribe 200 y copia X-User-Id al response para que el test pueda inspeccionarlo.
func nextHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Captured-User-Id", r.Header.Get("X-User-Id"))
	w.WriteHeader(http.StatusOK)
}

// ─── AuthValidator ────────────────────────────────────────────────────────────

func TestAuthValidator(t *testing.T) {
	validator := middleware.AuthValidator(testSecret)
	handler := validator(http.HandlerFunc(nextHandler))

	tests := []struct {
		name       string
		buildAuth  func(t *testing.T) string // devuelve el valor del header Authorization
		wantStatus int
		wantUserID string // solo verificado cuando wantStatus == 200
	}{
		{
			name:       "sin header Authorization",
			buildAuth:  func(t *testing.T) string { return "" },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "token sin prefijo Bearer",
			buildAuth: func(t *testing.T) string {
				// Token válido pero enviado sin "Bearer " delante
				return makeToken(t, validClaims("user-1"), testSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "prefijo Bearer pero token vacío",
			buildAuth:  func(t *testing.T) string { return "Bearer " },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "token firmado con secret incorrecto",
			buildAuth: func(t *testing.T) string {
				return "Bearer " + makeToken(t, validClaims("user-1"), differentSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "token expirado",
			buildAuth: func(t *testing.T) string {
				return "Bearer " + makeToken(t, expiredClaims("user-1"), testSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "token válido sin claim sub",
			buildAuth: func(t *testing.T) string {
				return "Bearer " + makeToken(t, jwt.MapClaims{
					"exp": time.Now().Add(1 * time.Hour).Unix(),
					// sin "sub"
				}, testSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "claim sub es número en lugar de string (type assertion falla)",
			buildAuth: func(t *testing.T) string {
				return "Bearer " + makeToken(t, jwt.MapClaims{
					"sub": 99999, // número, no string — la type assertion devuelve ok=false
					"exp": time.Now().Add(1 * time.Hour).Unix(),
				}, testSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "happy path — token válido completo",
			buildAuth: func(t *testing.T) string {
				return "Bearer " + makeToken(t, validClaims("user-abc-123"), testSecret, jwt.SigningMethodHS256)
			},
			wantStatus: http.StatusOK,
			wantUserID: "user-abc-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/any", nil)

			if authVal := tc.buildAuth(t); authVal != "" {
				req.Header.Set("Authorization", authVal)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}

			// En el happy path verificamos que X-User-Id llega al siguiente handler
			if tc.wantStatus == http.StatusOK && tc.wantUserID != "" {
				got := rr.Header().Get("X-Captured-User-Id")
				if got != tc.wantUserID {
					t.Errorf("X-User-Id propagado: got %q, want %q", got, tc.wantUserID)
				}
			}
		})
	}
}

// TestAuthValidator_AlgorithmNone verifica el ataque de sustitución de algoritmo.
// Un payload malicioso puede llegar con alg=none para evadir la verificación de firma.
// El middleware debe rechazarlo porque el type assertion a *jwt.SigningMethodHMAC falla.
func TestAuthValidator_AlgorithmNone(t *testing.T) {
	unsafeToken := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenStr, err := unsafeToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("no se pudo generar token con alg=none: %v", err)
	}

	validator := middleware.AuthValidator(testSecret)
	handler := validator(http.HandlerFunc(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("alg=none attack: got %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestAuthValidator_NextHandlerNotCalledOnFailure verifica que en cualquier caso
// de error el request se corta en el middleware y el siguiente handler no se ejecuta.
// Esto protege contra fugas accidentales de X-User-Id en respuestas de error.
func TestAuthValidator_NextHandlerNotCalledOnFailure(t *testing.T) {
	nextCalled := false
	spy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	validator := middleware.AuthValidator(testSecret)
	handler := validator(spy)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Sin Authorization — debe retornar 401 sin llamar al spy
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401, got %d", rr.Code)
	}
	if nextCalled {
		t.Error("el siguiente handler no debe ejecutarse cuando el token es inválido")
	}
}

// ─── RequireRole ──────────────────────────────────────────────────────────────

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name         string
		requiredRole string
		rolesHeader  string // valor del header X-User-Roles; "" significa que no se setea
		wantStatus   int
	}{
		{
			name:         "header X-User-Roles ausente",
			requiredRole: "ADMIN",
			rolesHeader:  "",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "rol requerido ausente — lista de un elemento",
			requiredRole: "ADMIN",
			rolesHeader:  "INQUILINO",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "rol requerido ausente — lista de varios elementos",
			requiredRole: "ADMIN",
			rolesHeader:  "INQUILINO,PROPIETARIO",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "coincidencia parcial de nombre no cuenta",
			requiredRole: "ADMIN",
			rolesHeader:  "SUPER_ADMIN,CO_ADMIN",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "case sensitive — minúscula no matchea mayúscula",
			requiredRole: "ADMIN",
			rolesHeader:  "admin",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "rol presente como único elemento",
			requiredRole: "ADMIN",
			rolesHeader:  "ADMIN",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "rol presente al inicio de la lista",
			requiredRole: "INQUILINO",
			rolesHeader:  "INQUILINO,PROPIETARIO,ADMIN",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "rol presente al final de la lista",
			requiredRole: "PROPIETARIO",
			rolesHeader:  "INQUILINO,ADMIN,PROPIETARIO",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "rol presente con espacios alrededor — TrimSpace lo resuelve",
			requiredRole: "ADMIN",
			rolesHeader:  "INQUILINO, ADMIN , PROPIETARIO",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roleMiddleware := middleware.RequireRole(tc.requiredRole)
			handler := roleMiddleware(http.HandlerFunc(nextHandler))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin", nil)
			if tc.rolesHeader != "" {
				req.Header.Set("X-User-Roles", tc.rolesHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

// TestRequireRole_ErrorBodyContainsRoleName verifica que el body del 403
// incluye el nombre del rol requerido para facilitar debugging en el cliente.
func TestRequireRole_ErrorBodyContainsRoleName(t *testing.T) {
	const role = "SUPER_ADMIN"
	roleMiddleware := middleware.RequireRole(role)
	handler := roleMiddleware(http.HandlerFunc(nextHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-Roles", "INQUILINO")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, role) {
		t.Errorf("body del 403 debería contener el rol %q, got: %s", role, body)
	}
}

// ─── Cadena AuthValidator → RequireRole ───────────────────────────────────────

// TestMiddlewareChain verifica la cadena completa tal como se usa en el router:
// AuthValidator envuelve a RequireRole envuelve al handler final.
func TestMiddlewareChain(t *testing.T) {
	validToken := makeToken(t, validClaims("user-xyz"), testSecret, jwt.SigningMethodHS256)
	invalidToken := "token.invalido.xxx"

	authMW := middleware.AuthValidator(testSecret)
	roleMW := middleware.RequireRole("PROPIETARIO")
	chain := authMW(roleMW(http.HandlerFunc(nextHandler)))

	tests := []struct {
		name        string
		authHeader  string
		rolesHeader string
		wantStatus  int
	}{
		{
			name:        "token válido + rol correcto → 200",
			authHeader:  "Bearer " + validToken,
			rolesHeader: "PROPIETARIO,INQUILINO",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "token válido + rol incorrecto → 403",
			authHeader:  "Bearer " + validToken,
			rolesHeader: "INQUILINO",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "token inválido — no llega al RequireRole → 401",
			authHeader:  "Bearer " + invalidToken,
			rolesHeader: "PROPIETARIO",
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "sin token ni roles → 401 (AuthValidator corta primero)",
			authHeader:  "",
			rolesHeader: "",
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.rolesHeader != "" {
				req.Header.Set("X-User-Roles", tc.rolesHeader)
			}
			rr := httptest.NewRecorder()

			chain.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rr.Code, tc.wantStatus)
			}
		})
	}
}

