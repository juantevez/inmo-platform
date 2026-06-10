package httpapi

import (
	"net/http"

	"inmo.platform/shared/pkg/health"
)

func NewRouter(propertyHandler *PropertyHandler, profileHandler *ProfileHandler, mediaHandler *MediaHandler, checker *health.Checker) http.Handler {
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("GET /healthz/live", checker.LiveHandler)
	mux.HandleFunc("GET /healthz/ready", checker.ReadyHandler)

	// Rutas de Propiedades
	mux.HandleFunc("GET /api/v1/properties", propertyHandler.List)
	mux.HandleFunc("POST /api/v1/properties", propertyHandler.Publish)
	mux.HandleFunc("PUT /api/v1/properties/{id}", propertyHandler.Update)
	mux.HandleFunc("POST /api/v1/properties/{id}/reserve", propertyHandler.Reserve)
	mux.HandleFunc("POST /api/v1/properties/{id}/quote", propertyHandler.Quote)

	// Media de Propiedades (fotos, videos, enlaces de redes sociales)
	mux.HandleFunc("POST /api/v1/properties/{id}/media/upload-url", mediaHandler.HandleGenerateUploadURL)
	mux.HandleFunc("POST /api/v1/properties/{id}/media", mediaHandler.HandleAddMedia)
	mux.HandleFunc("GET /api/v1/properties/{id}/media", mediaHandler.HandleListMedia)

	// Perfiles de negocio
	mux.HandleFunc("GET /api/v1/catalog/profiles/me", profileHandler.HandleGetProfile)
	mux.HandleFunc("POST /api/v1/catalog/profiles", profileHandler.HandleCreateOrUpdate)

	return mux
}
