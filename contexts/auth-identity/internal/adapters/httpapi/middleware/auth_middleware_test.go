package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"inmo.platform/contexts/auth-identity/internal/adapters/httpapi/middleware"
)

const testSecret = "test-secret-super-larga-para-hmac"

// ─── Helpers ────────────────────────────────────────────────────────────────

// signToken firma un JWT HS256 válido con los claims dados.
func signToken(t *testing.T, secret string, sub string, roles, perms []string, expiresAt time.Time) string {
	t.Helper()
	claims := middleware.AuthClaims{
		Sub:         sub,
		Roles:       roles,
		Permissions: perms,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("signToken: %v", err)
	}
	return signed
}

// signNoneAlgToken firma un JWT usando alg "none" (sin firma), simulando el ataque
// clásico de JWT donde un atacante fuerza el algoritmo "none" para bypassear la verificación.
func signNoneAlgToken(t *testing.T, sub string) string {
	t.Helper()
	claims := middleware.AuthClaims{
		Sub: sub,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodNone, claims)
	signed, err := token.SignedString(jwtlib.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("signNoneAlgToken: %v", err)
	}
	return signed
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodeError: no se pudo decodificar el body %q: %v", rec.Body.String(), err)
	}
	return got["error"]
}

// ─── JWTMiddleware ──────────────────────────────────────────────────────────

func TestJWTMiddleware_SinHeaderAuthorization_Retorna401(t *testing.T) {
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_HeaderSinPrefijoBearer_Retorna401(t *testing.T) {
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_BearerSinToken_Retorna401(t *testing.T) {
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_TokenMalformado_Retorna401(t *testing.T) {
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer esto-no-es-un-jwt")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, rec); got != "Token inválido o expirado" {
		t.Errorf("error: got %q, want %q", got, "Token inválido o expirado")
	}
}

func TestJWTMiddleware_FirmaInvalida_Retorna401(t *testing.T) {
	tokenStr := signToken(t, "otro-secret-distinto", "user-1", nil, nil, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_TokenExpirado_Retorna401(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", nil, nil, time.Now().Add(-1*time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_AlgoritmoNone_RechazaAtaqueDeBypass(t *testing.T) {
	// Ataque clásico de JWT: forzar alg=none para que el token "parezca" válido sin firma.
	// El middleware debe rechazarlo explícitamente en el keyFunc, no solo por casualidad.
	tokenStr := signNoneAlgToken(t, "user-1")
	handler := middleware.JWTMiddleware(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d (el ataque alg=none debe ser rechazado)", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_TokenValido_InyectaContextoYContinua(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-42", []string{"PROPIETARIO"}, []string{"property:create"}, time.Now().Add(time.Hour))

	var gotUserID string
	var gotRoles, gotPerms []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = middleware.UserIDFromContext(r.Context())
		gotRoles = middleware.RolesFromContext(r.Context())
		gotPerms = middleware.PermissionsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.JWTMiddleware(testSecret)(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "user-42" {
		t.Errorf("UserIDFromContext: got %q, want %q", gotUserID, "user-42")
	}
	if len(gotRoles) != 1 || gotRoles[0] != "PROPIETARIO" {
		t.Errorf("RolesFromContext: got %v, want [PROPIETARIO]", gotRoles)
	}
	if len(gotPerms) != 1 || gotPerms[0] != "property:create" {
		t.Errorf("PermissionsFromContext: got %v, want [property:create]", gotPerms)
	}
}

// ─── RequirePermission ──────────────────────────────────────────────────────

func TestRequirePermission_SinPermisosEnContexto_Retorna403(t *testing.T) {
	// Simula el uso incorrecto: RequirePermission encadenado sin pasar antes por JWTMiddleware.
	handler := middleware.RequirePermission("property:create")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/properties", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_PermisoFaltante_Retorna403(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", nil, []string{"property:read"}, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequirePermission("property:create")(okHandler()))
	req := httptest.NewRequest(http.MethodPost, "/properties", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermission_PermisoPresente_ContinuaAlSiguienteHandler(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", nil, []string{"property:create"}, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequirePermission("property:create")(okHandler()))
	req := httptest.NewRequest(http.MethodPost, "/properties", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── RequireAnyPermission ───────────────────────────────────────────────────

func TestRequireAnyPermission_SinPermisosEnContexto_Retorna403(t *testing.T) {
	handler := middleware.RequireAnyPermission("contract:read", "contract:update")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/contracts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAnyPermission_NingunoPresente_Retorna403(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", nil, []string{"property:read"}, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequireAnyPermission("contract:read", "contract:update")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/contracts", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireAnyPermission_UnoPresente_ContinuaAlSiguienteHandler(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", nil, []string{"contract:update"}, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequireAnyPermission("contract:read", "contract:update")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/contracts", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── RequireRole ────────────────────────────────────────────────────────────

func TestRequireRole_SinRolesEnContexto_Retorna403(t *testing.T) {
	handler := middleware.RequireRole("ROOT")(okHandler())
	req := httptest.NewRequest(http.MethodPost, "/tenants", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequireRole_RolFaltante_Retorna403(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", []string{"INQUILINO"}, nil, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequireRole("ROOT")(okHandler()))
	req := httptest.NewRequest(http.MethodPost, "/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := decodeError(t, rec); got != "Rol requerido: ROOT" {
		t.Errorf("error: got %q, want %q", got, "Rol requerido: ROOT")
	}
}

func TestRequireRole_RolPresente_ContinuaAlSiguienteHandler(t *testing.T) {
	tokenStr := signToken(t, testSecret, "user-1", []string{"ROOT"}, nil, time.Now().Add(time.Hour))
	handler := middleware.JWTMiddleware(testSecret)(middleware.RequireRole("ROOT")(okHandler()))
	req := httptest.NewRequest(http.MethodPost, "/tenants", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// ─── Helpers de contexto ────────────────────────────────────────────────────

func TestUserIDFromContext_ContextoVacio_RetornaStringVacio(t *testing.T) {
	if got := middleware.UserIDFromContext(context.Background()); got != "" {
		t.Errorf("UserIDFromContext: got %q, want %q", got, "")
	}
}

func TestRolesFromContext_ContextoVacio_RetornaNil(t *testing.T) {
	if got := middleware.RolesFromContext(context.Background()); got != nil {
		t.Errorf("RolesFromContext: got %v, want nil", got)
	}
}

func TestPermissionsFromContext_ContextoVacio_RetornaNil(t *testing.T) {
	if got := middleware.PermissionsFromContext(context.Background()); got != nil {
		t.Errorf("PermissionsFromContext: got %v, want nil", got)
	}
}

func TestUserIDFromContext_ConValorInyectado_RetornaValor(t *testing.T) {
	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserID, "user-99")
	if got := middleware.UserIDFromContext(ctx); got != "user-99" {
		t.Errorf("UserIDFromContext: got %q, want %q", got, "user-99")
	}
}
