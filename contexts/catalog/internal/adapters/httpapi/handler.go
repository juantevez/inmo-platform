package httpapi

import (
	"encoding/json"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/shared/pkg/apperr"
	"net/http"
)

type PropertyHandler struct {
	publishUC     *application.PublishPropertyUseCase
	changeStateUC *application.ChangePropertyStateUseCase
	listUC        *application.ListPropertiesUseCase
}

func NewPropertyHandler(
	publishUC *application.PublishPropertyUseCase,
	changeStateUC *application.ChangePropertyStateUseCase,
	listUC *application.ListPropertiesUseCase,
) *PropertyHandler {
	return &PropertyHandler{
		publishUC:     publishUC,
		changeStateUC: changeStateUC,
		listUC:        listUC,
	}
}

// Request para publicar propiedad
type PublishRequest struct {
	ID          string  `json:"id"`
	OwnerID     string  `json:"owner_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Address     string  `json:"address"`
}

func (h *PropertyHandler) Publish(w http.ResponseWriter, r *http.Request) {
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	dto := application.PublishPropertyDTO{
		ID:          req.ID,
		OwnerID:     req.OwnerID,
		Title:       req.Title,
		Description: req.Description,
		Price:       req.Price,
		Currency:    req.Currency,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Address:     req.Address,
	}

	if err := h.publishUC.Execute(r.Context(), dto); err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "success", "message": "propiedad publicada exitosamente"}`))
}

func (h *PropertyHandler) Reserve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id") // Característica nativa de Go 1.22+ para capturar URL params
	if id == "" {
		h.errorResponse(w, apperr.NewBadRequest("el ID de la propiedad es requerido", nil))
		return
	}

	if err := h.changeStateUC.Execute(r.Context(), id, application.ActionReserve); err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success", "message": "propiedad reservada"}`))
}

func (h *PropertyHandler) List(w http.ResponseWriter, r *http.Request) {
	properties, err := h.listUC.Execute(r.Context())
	if err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(properties)
}

// Helper para unificar las respuestas de error usando nuestro Shared Kernel
func (h *PropertyHandler) errorResponse(w http.ResponseWriter, err error) {
	statusCode := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// Si es un AppError conocido, exponemos su estructura limpia
	if appErr, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(appErr)
		return
	}

	// Si es un error genérico no controlado
	_, _ = w.Write([]byte(`{"type": "INTERNAL_SERVER_ERROR", "message": "un error inesperado ocurrió"}`))
}
