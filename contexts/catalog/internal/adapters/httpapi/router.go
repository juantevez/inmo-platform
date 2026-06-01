package httpapi

import "net/http"

func NewRouter(propertyHandler *PropertyHandler, profileHandler *ProfileHandler, mediaHandler *MediaHandler) http.Handler {
	mux := http.NewServeMux()

	// Rutas de Propiedades
	mux.HandleFunc("GET /api/v1/properties", propertyHandler.List)
	mux.HandleFunc("POST /api/v1/properties", propertyHandler.Publish)
	mux.HandleFunc("POST /api/v1/properties/{id}/reserve", propertyHandler.Reserve)

	// Media de Propiedades (fotos, videos, enlaces de redes sociales)
	mux.HandleFunc("POST /api/v1/properties/{id}/media/upload-url", mediaHandler.HandleGenerateUploadURL)
	mux.HandleFunc("POST /api/v1/properties/{id}/media", mediaHandler.HandleAddMedia)
	mux.HandleFunc("GET /api/v1/properties/{id}/media", mediaHandler.HandleListMedia)

	// Perfiles de negocio
	mux.HandleFunc("GET /api/v1/catalog/profiles/me", profileHandler.HandleGetProfile)
	mux.HandleFunc("POST /api/v1/catalog/profiles", profileHandler.HandleCreateOrUpdate)

	return mux
}
