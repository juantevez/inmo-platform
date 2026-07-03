package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/shared/pkg/apperr"
)

// LeadHandler expone el seguimiento manual del agente sobre un Lead.
// La identidad ya fue validada por el API Gateway (headers X-User-Id/etc);
// este handler no hace su propia validación de JWT.
type LeadHandler struct {
	getUC      *application.GetLeadUseCase
	contactUC  *application.ContactLeadUseCase
	scheduleUC *application.ScheduleVisitUseCase
	closeUC    *application.CloseLeadUseCase
}

func NewLeadHandler(
	getUC *application.GetLeadUseCase,
	contactUC *application.ContactLeadUseCase,
	scheduleUC *application.ScheduleVisitUseCase,
	closeUC *application.CloseLeadUseCase,
) *LeadHandler {
	return &LeadHandler{
		getUC:      getUC,
		contactUC:  contactUC,
		scheduleUC: scheduleUC,
		closeUC:    closeUC,
	}
}

// GET /api/v1/leads/{id}
func (h *LeadHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	dto, err := h.getUC.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// POST /api/v1/leads/{id}/contact
func (h *LeadHandler) HandleContact(w http.ResponseWriter, r *http.Request) {
	dto, err := h.contactUC.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// POST /api/v1/leads/{id}/schedule-visit
func (h *LeadHandler) HandleScheduleVisit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VisitAt string `json:"visit_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	visitAt, err := time.Parse(time.RFC3339, req.VisitAt)
	if err != nil {
		h.errResp(w, apperr.NewBadRequest("visit_at inválida (formato RFC3339)", err))
		return
	}

	dto, err := h.scheduleUC.Execute(r.Context(), application.ScheduleVisitDTO{
		LeadID:  r.PathValue("id"),
		VisitAt: visitAt,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// POST /api/v1/leads/{id}/close
func (h *LeadHandler) HandleClose(w http.ResponseWriter, r *http.Request) {
	dto, err := h.closeUC.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

func (h *LeadHandler) errResp(w http.ResponseWriter, err error) {
	code := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if ae, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(ae)
		return
	}
	_, _ = w.Write([]byte(`{"type":"INTERNAL_SERVER_ERROR","message":"error inesperado"}`))
}
