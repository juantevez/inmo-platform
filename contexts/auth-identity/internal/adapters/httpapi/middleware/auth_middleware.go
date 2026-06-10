package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// contextKey es un tipo privado para evitar colisiones de keys en el contexto HTTP
type contextKey string

const (
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyRoles       contextKey = "roles"
	ContextKeyPermissions contextKey = "permissions"
)

// AuthClaims representa los claims que espera este middleware en el JWT.
// Debe coincidir con lo que GenerateAccessToken emite en main.go.
type AuthClaims struct {
	Sub         string   `json:"sub"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	jwtlib.RegisteredClaims
}

// JWTMiddleware valida la firma del token y lo rechaza si:
//   - No viene el header Authorization: Bearer <token>
//   - La firma es inválida o el token expiró
//   - El token está malformado
//
// Si el token es válido, inyecta user_id, roles y permissions en el contexto
// para que los handlers downstream los lean sin volver a parsear el JWT.
func JWTMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	secretBytes := []byte(jwtSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				respondUnauthorized(w, "Authorization header requerido (Bearer <token>)")
				return
			}

			claims, err := parseAndValidate(tokenStr, secretBytes)
			if err != nil {
				respondUnauthorized(w, "Token inválido o expirado")
				return
			}

			// Inyectamos los datos del JWT en el contexto para los handlers
			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.Sub)
			ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
			ctx = context.WithValue(ctx, ContextKeyPermissions, claims.Permissions)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission es un middleware que verifica que el usuario tenga UN permiso específico.
// Se encadena DESPUÉS de JWTMiddleware.
//
// Uso en el router:
//
//	mux.Handle("POST /api/v1/properties",
//	    JWTMiddleware(secret)(
//	        RequirePermission("property:create")(handleCreateProperty)
//	    ))
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permissions, ok := r.Context().Value(ContextKeyPermissions).([]string)
			if !ok {
				respondForbidden(w, "No se pudieron leer los permisos del token")
				return
			}

			if !hasPermission(permissions, permission) {
				respondForbidden(w, "No tenés permiso para realizar esta acción ("+permission+")")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission verifica que el usuario tenga AL MENOS UNO de los permisos listados.
// Útil para endpoints accesibles por múltiples roles.
//
// Uso: RequireAnyPermission("contract:read", "contract:update")
func RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userPerms, ok := r.Context().Value(ContextKeyPermissions).([]string)
			if !ok {
				respondForbidden(w, "No se pudieron leer los permisos del token")
				return
			}

			for _, required := range permissions {
				if hasPermission(userPerms, required) {
					next.ServeHTTP(w, r)
					return
				}
			}

			respondForbidden(w, "No tenés permiso para realizar esta acción")
			//return
		})
	}
}

// RequireRole verifica que el usuario tenga un rol específico.
// Preferir RequirePermission cuando sea posible — es más granular.
// Usar RequireRole solo para lógica que dependa estrictamente del tipo de actor
// (ej: solo ROOT puede crear tenants).
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles, ok := r.Context().Value(ContextKeyRoles).([]string)
			if !ok {
				respondForbidden(w, "No se pudieron leer los roles del token")
				return
			}

			for _, userRole := range roles {
				if userRole == role {
					next.ServeHTTP(w, r)
					return
				}
			}

			respondForbidden(w, "Rol requerido: "+role)
			//return
		})
	}
}

// --- Helpers para los handlers ---

// UserIDFromContext extrae el user_id del contexto.
// Retorna "" si el middleware JWT no procesó el request.
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyUserID).(string)
	return id
}

// RolesFromContext extrae los roles del contexto.
func RolesFromContext(ctx context.Context) []string {
	roles, _ := ctx.Value(ContextKeyRoles).([]string)
	return roles
}

// PermissionsFromContext extrae los permisos del contexto.
func PermissionsFromContext(ctx context.Context) []string {
	perms, _ := ctx.Value(ContextKeyPermissions).([]string)
	return perms
}

// --- Funciones internas ---

func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func parseAndValidate(tokenStr string, secret []byte) (*AuthClaims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &AuthClaims{}, func(token *jwtlib.Token) (interface{}, error) {
		// Validar que el algoritmo sea HS256 — rechazar cualquier otro (incluido "none")
		if _, ok := token.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, jwtlib.ErrSignatureInvalid
		}
		return secret, nil
	})

	if err != nil || !token.Valid {
		return nil, err
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok {
		return nil, jwtlib.ErrTokenInvalidClaims
	}

	return claims, nil
}

func hasPermission(userPerms []string, required string) bool {
	for _, p := range userPerms {
		if p == required {
			return true
		}
	}
	return false
}

func respondUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func respondForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
