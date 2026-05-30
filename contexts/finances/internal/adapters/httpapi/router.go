package httpapi

import (
	"net/http"
)

// RegisterSettlementRoutes mapea las urls contra las funciones del controlador
func RegisterSettlementRoutes(mux *http.ServeMux, handler *SettlementHandler) {
	// Definición de endpoints semánticos de finanzas
	mux.HandleFunc("/api/v1/settlements/create", handler.HandleCreate)
	mux.HandleFunc("/api/v1/settlements/concepts/add", handler.HandleAddConcept)
	mux.HandleFunc("/api/v1/settlements/close", handler.HandleClose)
}
