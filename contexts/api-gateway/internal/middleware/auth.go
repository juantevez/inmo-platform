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

			// Supongamos que del token extraemos que el usuario es "user-123" y tiene rol "TENANT"
			// Inyectamos estos datos en los Headers para que los microservicios de atrás no tengan que re-validar el JWT
			r.Header.Set("X-User-Id", "user-123")
			r.Header.Set("X-User-Role", "TENANT")

			next.ServeHTTP(w, r)
		})
	}
}
