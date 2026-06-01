package httpapi

import "net/http"

func NewRouter(propertyHandler *PropertyHandler, profileHandler *ProfileHandler) http.Handler {
	mux := http.NewServeMux()

	// 🏠 Rutas de Propiedades (Existentes)
	mux.HandleFunc("GET /api/v1/properties", propertyHandler.List)
	mux.HandleFunc("POST /api/v1/properties", propertyHandler.Publish)
	mux.HandleFunc("POST /api/v1/properties/{id}/reserve", propertyHandler.Reserve)

	// 👤 Perfiles de negocio
	mux.HandleFunc("GET /api/v1/catalog/profiles/me", profileHandler.HandleGetProfile)
	mux.HandleFunc("POST /api/v1/catalog/profiles", profileHandler.HandleCreateOrUpdate)

	return mux
}
