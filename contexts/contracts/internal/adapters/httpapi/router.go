package httpapi

import (
	"net/http"

	"inmo.platform/shared/pkg/health"
)

func NewRouter(h *ContractHandler, rh *ReservationHandler, checker *health.Checker) http.Handler {
	mux := http.NewServeMux()

	// Contratos inmobiliarios (tradicional)
	mux.HandleFunc("POST /api/v1/contracts", h.Create)
	mux.HandleFunc("POST /api/v1/contracts/activate", h.Activate)

	// Panel del propietario — ANTES de /reservations/{id} para evitar conflicto de routing
	mux.HandleFunc("GET /api/v1/reservations/owner", rh.HandleListOwner)

	// Reservas de alquiler temporario
	mux.HandleFunc("POST /api/v1/reservations", rh.HandleCreate)
	mux.HandleFunc("GET /api/v1/reservations/{id}", rh.HandleGet)
	mux.HandleFunc("POST /api/v1/reservations/{id}/confirm", rh.HandleConfirm)
	mux.HandleFunc("POST /api/v1/reservations/{id}/cancel", rh.HandleCancel)

	// Health checks
	mux.HandleFunc("GET /health/live", checker.LiveHandler)
	mux.HandleFunc("GET /health/ready", checker.ReadyHandler)

	return mux
}
