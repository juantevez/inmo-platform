package httpapi

import (
	"net/http"
)

// NewRouter ensambla las rutas del Bounded Context usando las mejoras de ruteo de Go 1.22+
func NewRouter(handler *AuthHandler) http.Handler {
	mux := http.NewServeMux()

	// Endpoints Públicos de Identidad y Autenticación
	mux.HandleFunc("POST /auth/register", handler.HandleRegister)
	mux.HandleFunc("POST /auth/login", handler.HandleLoginPassword)
	mux.HandleFunc("GET /auth/verify", handler.HandleVerifyEmail)

	// Los handlers de SSO y OTPs se irán acoplando de la misma forma acá abajo...
	// mux.HandleFunc("POST /auth/sso/google", handler.HandleGoogleLogin)

	return mux
}
