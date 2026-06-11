package middleware

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func AuthValidator(jwtSecret string) func(http.Handler) http.Handler {
	secret := []byte(jwtSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"No autorizado","message":"Token faltante o inválido"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"No autorizado","message":"Token expirado o inválido"}`, http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, `{"error":"No autorizado","message":"Claims inválidos"}`, http.StatusUnauthorized)
				return
			}

			userID, _ := claims["sub"].(string)
			if userID == "" {
				http.Error(w, `{"error":"No autorizado","message":"Token sin identidad de usuario"}`, http.StatusUnauthorized)
				return
			}

			// Inyectar identidad y permisos como headers para los backends.
			// Los servicios downstream leen estos headers en lugar de parsear
			// el JWT nuevamente — un solo punto de validación en el gateway.
			roles, _ := claims["roles"].([]interface{})
			permissions, _ := claims["permissions"].([]interface{})

			r.Header.Set("X-User-Id", userID)
			r.Header.Set("X-User-Roles", joinClaims(roles))
			r.Header.Set("X-Permissions", joinClaims(permissions))

			next.ServeHTTP(w, r)
		})
	}
}

// joinClaims convierte un slice de interface{} (como vienen los arrays del JWT)
// a un string separado por comas: ["property:create","property:read"] → "property:create,property:read"
func joinClaims(vals []interface{}) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ",")
}

// RequireRole middleware complementario para validar roles específicos.
// Funciona con el header X-User-Roles inyectado por AuthValidator.
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rolesHeader := r.Header.Get("X-User-Roles")
			if rolesHeader == "" {
				http.Error(w, `{"error":"Prohibido","message":"No tenés los permisos necesarios"}`, http.StatusForbidden)
				return
			}

			for _, role := range strings.Split(rolesHeader, ",") {
				if strings.TrimSpace(role) == requiredRole {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, `{"error":"Prohibido","message":"Se requiere el rol de `+requiredRole+` para realizar esta acción"}`, http.StatusForbidden)
		})
	}
}

// RequirePermission middleware para validar permisos granulares.
// Funciona con el header X-Permissions inyectado por AuthValidator.
// Usar en lugar de RequireRole para control más fino (ej: "property:create").
func RequirePermission(requiredPermission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permsHeader := r.Header.Get("X-Permissions")
			if permsHeader == "" {
				http.Error(w, `{"error":"Prohibido","message":"Sin permisos asignados"}`, http.StatusForbidden)
				return
			}

			for _, perm := range strings.Split(permsHeader, ",") {
				if strings.TrimSpace(perm) == requiredPermission {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, `{"error":"Prohibido","message":"Permiso requerido: `+requiredPermission+`"}`, http.StatusForbidden)
		})
	}
}
