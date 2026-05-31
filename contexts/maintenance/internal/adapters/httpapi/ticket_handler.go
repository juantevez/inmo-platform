package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"inmo.platform/contexts/maintenance/internal/application"
	"inmo.platform/contexts/maintenance/internal/domain"
)

type TicketHandler struct {
	createUseCase  *application.CreateTicketUseCase
	assignUseCase  *application.AssignProviderUseCase
	submitUseCase  *application.SubmitQuoteUseCase
	approveUseCase *application.ApproveTicketUseCase
	closeUseCase   *application.CloseTicketUseCase
}

func NewTicketHandler(
	create *application.CreateTicketUseCase,
	assign *application.AssignProviderUseCase,
	submit *application.SubmitQuoteUseCase,
	approve *application.ApproveTicketUseCase,
	closeUC *application.CloseTicketUseCase,
) *TicketHandler {
	return &TicketHandler{
		createUseCase:  create,
		assignUseCase:  assign,
		submitUseCase:  submit,
		approveUseCase: approve,
		closeUseCase:   closeUC,
	}
}

// 1. POST /api/v1/tickets/report
func (h *TicketHandler) ReportTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var cmd application.CreateTicketCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.createUseCase.Execute(r.Context(), cmd); err != nil {
		if errors.Is(err, application.ErrPropertyNotFound) {
			h.respondWithError(w, http.StatusNotFound, err.Error())
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondWithJSON(w, http.StatusCreated, map[string]string{"message": "Incidencia reportada con éxito", "id": cmd.ID})
}

// 2. POST /api/v1/tickets/assign
func (h *TicketHandler) AssignProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var cmd application.AssignProviderCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.assignUseCase.Execute(r.Context(), cmd); err != nil {
		h.handleDomainError(w, err)
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "Proveedor asignado y ticket validado"})
}

// 3. POST /api/v1/tickets/quote
func (h *TicketHandler) SubmitQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var cmd application.SubmitQuoteCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.submitUseCase.Execute(r.Context(), cmd); err != nil {
		h.handleDomainError(w, err)
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "Presupuesto cargado exitosamente"})
}

// 4. POST /api/v1/tickets/approve
func (h *TicketHandler) ApproveTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.approveUseCase.Execute(r.Context(), req.TicketID); err != nil {
		h.handleDomainError(w, err)
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "Ticket aprobado. Obra iniciada (IN_PROGRESS)"})
}

// 5. POST /api/v1/tickets/close
func (h *TicketHandler) CloseTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var cmd application.CloseTicketCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.closeUseCase.Execute(r.Context(), cmd); err != nil {
		h.handleDomainError(w, err)
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "Incidencia finalizada y ticket cerrado de forma conforme"})
}

// Helpers de respuesta estandarizados
func (h *TicketHandler) handleDomainError(w http.ResponseWriter, err error) {
	if errors.Is(err, application.ErrTicketNotFound) {
		h.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, domain.ErrInvalidStatusTransition) ||
		errors.Is(err, domain.ErrProviderRequired) ||
		errors.Is(err, domain.ErrInvalidQuoteAmount) ||
		errors.Is(err, domain.ErrEvidenceRequired) {
		h.respondWithError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	h.respondWithError(w, http.StatusInternalServerError, err.Error())
}

func (h *TicketHandler) respondWithError(w http.ResponseWriter, code int, msg string) {
	h.respondWithJSON(w, code, map[string]string{"error": msg})
}

func (h *TicketHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
