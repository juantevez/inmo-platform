package httpapi

import "net/http"

func NewRouter(handler *PropertyHandler) *http.ServeMux {
	mux := http.NewServeMux()

	// Mapeo de rutas usando el enrutamiento moderno de Go net/http
	mux.HandleFunc("POST /api/v1/properties", handler.Publish)
	mux.HandleFunc("POST /api/v1/properties/{id}/reserve", handler.Reserve)

	return mux
}
