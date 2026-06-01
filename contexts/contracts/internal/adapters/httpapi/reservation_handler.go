package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/shared/pkg/apperr"
)

type ReservationHandler struct {
	createUC  *application.CreateReservationUseCase
	confirmUC *application.ConfirmReservationUseCase
	cancelUC  *application.CancelReservationUseCase
	getUC     *application.GetReservationUseCase
}

func NewReservationHandler(
	createUC *application.CreateReservationUseCase,
	confirmUC *application.ConfirmReservationUseCase,
	cancelUC *application.CancelReservationUseCase,
	getUC *application.GetReservationUseCase,
) *ReservationHandler {
	return &ReservationHandler{createUC: createUC, confirmUC: confirmUC, cancelUC: cancelUC, getUC: getUC}
}

// POST /api/v1/reservations
func (h *ReservationHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-User-Id")
	if tenantID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	var req struct {
		PropertyID   string `json:"property_id"`
		CheckInDate  string `json:"check_in_date"`
		CheckOutDate string `json:"check_out_date"`
		GuestMessage string `json:"guest_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	checkIn, err := time.Parse("2006-01-02", req.CheckInDate)
	if err != nil {
		h.errResp(w, apperr.NewBadRequest("check_in_date inválida (formato YYYY-MM-DD)", err))
		return
	}
	checkOut, err := time.Parse("2006-01-02", req.CheckOutDate)
	if err != nil {
		h.errResp(w, apperr.NewBadRequest("check_out_date inválida (formato YYYY-MM-DD)", err))
		return
	}

	dto, err := h.createUC.Execute(r.Context(), application.CreateReservationCommand{
		PropertyID:   req.PropertyID,
		TenantID:     tenantID,
		CheckInDate:  checkIn,
		CheckOutDate: checkOut,
		GuestMessage: req.GuestMessage,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(dto)
}

// POST /api/v1/reservations/{id}/confirm
func (h *ReservationHandler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-Id")
	if ownerID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	dto, err := h.confirmUC.Execute(r.Context(), r.PathValue("id"), ownerID)
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

// POST /api/v1/reservations/{id}/cancel
func (h *ReservationHandler) HandleCancel(w http.ResponseWriter, r *http.Request) {
	requesterID := r.Header.Get("X-User-Id")
	if requesterID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista", nil))
		return
	}

	if err := h.cancelUC.Execute(r.Context(), r.PathValue("id"), requesterID); err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"cancelled"}`))
}

// GET /api/v1/reservations/{id}
func (h *ReservationHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	dto, err := h.getUC.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dto)
}

func (h *ReservationHandler) errResp(w http.ResponseWriter, err error) {
	code := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if ae, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(ae)
		return
	}
	_, _ = w.Write([]byte(`{"type":"INTERNAL_SERVER_ERROR","message":"error inesperado"}`))
}
