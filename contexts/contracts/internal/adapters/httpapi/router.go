package httpapi

import (
	"net/http"
)

func NewRouter(h *ContractHandler) http.Handler {
	mux := http.NewServeMux()

	// Rutas de contratos inmobiliarios
	mux.HandleFunc("/api/v1/contracts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.Create(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/contracts/activate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.Activate(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return mux
}
