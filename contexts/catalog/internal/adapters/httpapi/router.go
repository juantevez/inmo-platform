package httpapi

import "net/http"

func NewRouter(handler *PropertyHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/properties", handler.List)
	mux.HandleFunc("POST /api/v1/properties", handler.Publish)
	mux.HandleFunc("POST /api/v1/properties/{id}/reserve", handler.Reserve)

	return mux
}
