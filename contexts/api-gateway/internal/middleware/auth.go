package middleware

import (
	"net/http"
	"strings"
	// Aquí usarías una librería real de JWT como "github.com/golang-jwt/jwt/v5"
)

func AuthValidator(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error": "No autorizado", "message": "Token faltante o inválido"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			// 🔍 Simulación de validación de JWT (reemplazar con jwt.Parse en producción)
			if tokenStr == "invalido" {
				http.Error(w, `{"error": "No autorizado", "message": "Token expirado o corrupto"}`, http.StatusUnauthorized)
				return
			}

			// 🚀 Simulación de Claims obtenidos del JWT real.
			// Ahora simulamos un usuario que tiene múltiples roles asignados en la base de datos.
			userID := "user-123"
			userRoles := []string{"PROPIETARIO", "INQUILINO"}

			// Inyectamos los datos en los Headers de la request
			r.Header.Set("X-User-Id", userID)

			// Unimos el slice de strings en un único string separado por comas: "PROPIETARIO,INQUILINO"
			r.Header.Set("X-User-Roles", strings.Join(userRoles, ","))

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
