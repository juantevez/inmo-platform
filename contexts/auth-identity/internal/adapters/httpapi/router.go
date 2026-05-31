package httpapi

import (
	"net/http"
)

func NewRouter(handler *AuthHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /auth/register", handler.HandleRegister)
	mux.HandleFunc("POST /auth/login", handler.HandleLoginPassword)
	mux.HandleFunc("GET /auth/verify", handler.HandleVerifyEmail)
	mux.HandleFunc("POST /auth/sso/google", handler.HandleGoogleLogin)
	mux.HandleFunc("POST /auth/sso/meta", handler.HandleMetaLogin)

	return mux
}
