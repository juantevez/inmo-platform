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

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:5500")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
