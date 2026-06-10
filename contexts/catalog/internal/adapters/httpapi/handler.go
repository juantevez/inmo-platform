package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type PropertyHandler struct {
	publishUC     *application.PublishPropertyUseCase
	changeStateUC *application.ChangePropertyStateUseCase
	listUC        *application.ListPropertiesUseCase
	quoteUC       *application.QuotePropertyUseCase
	updateUC      *application.UpdatePropertyUseCase
}

func NewPropertyHandler(
	publishUC *application.PublishPropertyUseCase,
	changeStateUC *application.ChangePropertyStateUseCase,
	listUC *application.ListPropertiesUseCase,
	quoteUC *application.QuotePropertyUseCase,
	updateUC *application.UpdatePropertyUseCase,
) *PropertyHandler {
	return &PropertyHandler{
		publishUC:     publishUC,
		changeStateUC: changeStateUC,
		listUC:        listUC,
		quoteUC:       quoteUC,
		updateUC:      updateUC,
	}
}

// Request para publicar propiedad — owner_id lo provee el gateway vía X-User-Id
type PublishRequest struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Address       string  `json:"address"`
	OperationType string  `json:"operation_type"`
	PetPolicy     string  `json:"pet_policy"`
	// Campos de alquiler temporario
	Amenities       []domain.Amenity      `json:"amenities"`
	CheckInTime     string                `json:"check_in_time"`
	CheckOutTime    string                `json:"check_out_time"`
	MinNights       int                   `json:"min_nights"`
	MaxNights       int                   `json:"max_nights"`
	NightPrice      float64               `json:"night_price"`
	CleaningFee     float64               `json:"cleaning_fee"`
	SecurityDeposit float64               `json:"security_deposit"`
	PricingRules    []domain.PricingRule  `json:"pricing_rules"`
}

func (h *PropertyHandler) Publish(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-Id")
	if ownerID == "" {
		h.errorResponse(w, apperr.NewBadRequest("identidad del usuario no provista por el gateway", nil))
		return
	}

	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	dto := application.PublishPropertyDTO{
		ID:              req.ID,
		OwnerID:         ownerID,
		Title:           req.Title,
		Description:     req.Description,
		Price:           req.Price,
		Currency:        req.Currency,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		Address:         req.Address,
		OperationType:   req.OperationType,
		PetPolicy:       req.PetPolicy,
		Amenities:       req.Amenities,
		CheckInTime:     req.CheckInTime,
		CheckOutTime:    req.CheckOutTime,
		MinNights:       req.MinNights,
		MaxNights:       req.MaxNights,
		NightPrice:      req.NightPrice,
		CleaningFee:     req.CleaningFee,
		SecurityDeposit: req.SecurityDeposit,
		PricingRules:    req.PricingRules,
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
	q := r.URL.Query()

	filters := ports.ListFilters{
		State:         q.Get("status"),
		OperationType: q.Get("operation"),
		PetPolicy:     q.Get("pets"),
		Limit:         parseIntQuery(q.Get("limit"), 50),
		Offset:        parseIntQuery(q.Get("offset"), 0),
	}
	if v := q.Get("min_price"); v != "" {
		filters.MinPrice, _ = strconv.ParseFloat(v, 64)
	}
	if v := q.Get("max_price"); v != "" {
		filters.MaxPrice, _ = strconv.ParseFloat(v, 64)
	}

	response, err := h.listUC.Execute(r.Context(), filters)
	if err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func parseIntQuery(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// Quote: POST /api/v1/properties/{id}/quote
func (h *PropertyHandler) Quote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CheckInDate  string `json:"check_in_date"`
		CheckOutDate string `json:"check_out_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	checkIn, err := time.Parse("2006-01-02", req.CheckInDate)
	if err != nil {
		h.errorResponse(w, apperr.NewBadRequest("check_in_date inválida, usar formato YYYY-MM-DD", err))
		return
	}
	checkOut, err := time.Parse("2006-01-02", req.CheckOutDate)
	if err != nil {
		h.errorResponse(w, apperr.NewBadRequest("check_out_date inválida, usar formato YYYY-MM-DD", err))
		return
	}

	resp, err := h.quoteUC.Execute(r.Context(), application.QuoteCommand{
		PropertyID:   r.PathValue("id"),
		CheckInDate:  checkIn,
		CheckOutDate: checkOut,
	})
	if err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// Request para actualizar propiedad — owner_id lo provee el gateway vía X-User-Id
type UpdateRequest struct {
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	Currency    *string  `json:"currency,omitempty"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Address     *string  `json:"address,omitempty"`
	PetPolicy   *string  `json:"pet_policy,omitempty"`
	// Campos de alquiler temporario
	CheckInTime     *string               `json:"check_in_time,omitempty"`
	CheckOutTime    *string               `json:"check_out_time,omitempty"`
	MinNights       *int                  `json:"min_nights,omitempty"`
	MaxNights       *int                  `json:"max_nights,omitempty"`
	NightPrice      *float64              `json:"night_price,omitempty"`
	CleaningFee     *float64              `json:"cleaning_fee,omitempty"`
	SecurityDeposit *float64              `json:"security_deposit,omitempty"`
	PricingRules    []domain.PricingRule  `json:"pricing_rules,omitempty"`
}

func (h *PropertyHandler) Update(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-Id")
	if ownerID == "" {
		h.errorResponse(w, apperr.NewBadRequest("identidad del usuario no provista por el gateway", nil))
		return
	}

	propertyID := r.PathValue("id")
	if propertyID == "" {
		h.errorResponse(w, apperr.NewBadRequest("el ID de la propiedad es requerido", nil))
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	dto := application.UpdatePropertyDTO{
		Title:           req.Title,
		Description:     req.Description,
		Price:           req.Price,
		Currency:        req.Currency,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		Address:         req.Address,
		PetPolicy:       req.PetPolicy,
		CheckInTime:     req.CheckInTime,
		CheckOutTime:    req.CheckOutTime,
		MinNights:       req.MinNights,
		MaxNights:       req.MaxNights,
		NightPrice:      req.NightPrice,
		CleaningFee:     req.CleaningFee,
		SecurityDeposit: req.SecurityDeposit,
		PricingRules:    req.PricingRules,
	}

	if err := h.updateUC.Execute(r.Context(), propertyID, dto); err != nil {
		h.errorResponse(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "success", "message": "propiedad actualizada exitosamente"}`))
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
