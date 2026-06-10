package httpapi

import (
	"net/http"
)

func NewRouter(handler *AuthHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/register", handler.HandleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", handler.HandleLoginPassword)
	mux.HandleFunc("GET /api/v1/auth/verify", handler.HandleVerifyEmail)
	mux.HandleFunc("GET /api/v1/auth/sso/config", handler.HandleSSOConfig)
	mux.HandleFunc("POST /api/v1/auth/sso/google", handler.HandleGoogleLogin)
	mux.HandleFunc("POST /api/v1/auth/sso/meta", handler.HandleMetaLogin)

	return mux
}
