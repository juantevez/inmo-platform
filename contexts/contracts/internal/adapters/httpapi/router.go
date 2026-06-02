package httpapi

import (
	"net/http"
)

func NewRouter(h *ContractHandler, rh *ReservationHandler) http.Handler {
	mux := http.NewServeMux()

	// Contratos inmobiliarios (tradicional)
	mux.HandleFunc("POST /api/v1/contracts", h.Create)
	mux.HandleFunc("POST /api/v1/contracts/activate", h.Activate)

	// Panel del propietario — DEBE ir ANTES de /reservations/{id}
	// para que el ServeMux no lo trate como un {id} = "owner"
	mux.HandleFunc("GET /api/v1/reservations/owner", rh.HandleListOwner)

	// Reservas de alquiler temporario
	mux.HandleFunc("POST /api/v1/reservations", rh.HandleCreate)
	mux.HandleFunc("GET /api/v1/reservations/{id}", rh.HandleGet)
	mux.HandleFunc("POST /api/v1/reservations/{id}/confirm", rh.HandleConfirm)
	mux.HandleFunc("POST /api/v1/reservations/{id}/cancel", rh.HandleCancel)

	return mux
}
