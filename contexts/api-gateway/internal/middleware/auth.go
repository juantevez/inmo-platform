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

			r.Header.Set("X-User-Id", userID)
			next.ServeHTTP(w, r)
		})
	}
}

// 🔒 Middleware complementario para validar roles específicos en rutas puntuales del Gateway
func RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rolesHeader := r.Header.Get("X-User-Roles")

			// Si el header está vacío, el usuario directamente no pasó por AuthValidator o no tiene roles
			if rolesHeader == "" {
				http.Error(w, `{"error": "Prohibido", "message": "No tenés los permisos necesarios"}`, http.StatusForbidden)
				return
			}

			// Validamos si el rol requerido se encuentra en la lista
			roles := strings.Split(rolesHeader, ",")
			hasRole := false
			for _, role := range roles {
				if strings.TrimSpace(role) == requiredRole {
					hasRole = true
					break
				}
			}

			if !hasRole {
				http.Error(w, `{"error": "Prohibido", "message": "Se requiere el rol de `+requiredRole+` para realizar esta acción"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
