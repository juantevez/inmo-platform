package httpapi

import (
	"net/http"

	"inmo.platform/shared/pkg/health"
)

// NewRouter registra las rutas del módulo CRM. La autenticación ya la resolvió
// el API Gateway antes de proxyear el request (ver contexts/api-gateway),
// por eso no hay middleware de JWT local acá.
func NewRouter(h *LeadHandler, checker *health.Checker) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/leads/{id}", h.HandleGet)
	mux.HandleFunc("POST /api/v1/leads/{id}/contact", h.HandleContact)
	mux.HandleFunc("POST /api/v1/leads/{id}/schedule-visit", h.HandleScheduleVisit)
	mux.HandleFunc("POST /api/v1/leads/{id}/close", h.HandleClose)

	mux.HandleFunc("GET /health/live", checker.LiveHandler)
	mux.HandleFunc("GET /health/ready", checker.ReadyHandler)

	return mux
}
