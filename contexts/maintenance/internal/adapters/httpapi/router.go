package httpapi

import (
	"net/http"
)

// MapTicketRoutes registra los endpoints del ciclo de vida del Ticket en el ServeMux
func MapTicketRoutes(mux *http.ServeMux, handler *TicketHandler) {
	mux.HandleFunc("/api/v1/tickets/report", handler.ReportTicket)
	mux.HandleFunc("/api/v1/tickets/assign", handler.AssignProvider)
	mux.HandleFunc("/api/v1/tickets/quote", handler.SubmitQuote)
	mux.HandleFunc("/api/v1/tickets/approve", handler.ApproveTicket)
	mux.HandleFunc("/api/v1/tickets/close", handler.CloseTicket)
}
