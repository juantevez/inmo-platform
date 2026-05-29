package httpapi

import (
	"inmo.platform/contexts/contracts/internal/application"
	"encoding/json"
	"net/http"
	"time"

	"inmo.platform/shared/pkg/apperr"
)

type ContractHandler struct {
	createUC   *application.CreateContractUseCase
	activateUC *application.ActivateContractUseCase
}

func NewContractHandler(create *application.CreateContractUseCase, activate *application.ActivateContractUseCase) *ContractHandler {
	return &ContractHandler{createUC: create, activateUC: activate}
}

type createContractReq struct {
	ID               string    `json:"id"`
	PropertyID       string    `json:"property_id"`
	TenantID         string    `json:"tenant_id"`
	OwnerID          string    `json:"owner_id"`
	Amount           float64   `json:"amount"`
	Currency         string    `json:"currency"`
	StartDate        time.Time `json:"start_date"`
	EndDate          time.Time `json:"end_date"`
	AdjustmentIndex  string    `json:"adjustment_index"`
	AdjustmentPeriod int       `json:"adjustment_period"`
}

func (h *ContractHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createContractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "JSON malformado"}`))
		return
	}

	dto := application.CreateContractDTO{
		ID:               req.ID,
		PropertyID:       req.PropertyID,
		TenantID:         req.TenantID,
		OwnerID:          req.OwnerID,
		Amount:           req.Amount,
		Currency:         req.Currency,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		AdjustmentIndex:  req.AdjustmentIndex,
		AdjustmentPeriod: req.AdjustmentPeriod,
	}

	if err := h.createUC.Execute(r.Context(), dto); err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "success", "message": "Contrato borrador creado con éxito"}`))
}

func (h *ContractHandler) Activate(w http.ResponseWriter, r *http.Request) {
	// Obtenemos el ID de la URL. Por simplicidad con net/http estándar usamos query param o mapeo simple
	contractID := r.URL.Query().Get("id")
	if contractID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "El parámetro 'id' es requerido"}`))
		return
	}

	if err := h.activateUC.Execute(r.Context(), contractID); err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success", "message": "Contrato firmado y activado de forma atómica"}`))
}

func (h *ContractHandler) errorResponse(w http.ResponseWriter, err error) {
	statusCode := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if appErr, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(appErr)
		return
	}

	_, _ = w.Write([]byte(`{"type": "INTERNAL_SERVER_ERROR", "message": "un error inesperado ocurrió"}`))
}
