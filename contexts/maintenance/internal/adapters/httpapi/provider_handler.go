package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"inmo.platform/contexts/maintenance/internal/application"
	"inmo.platform/contexts/maintenance/internal/domain"
)

type ProviderHandler struct {
	registerUC      *application.RegisterProviderUseCase
	listProvidersUC *application.ListProvidersUseCase
}

func NewProviderHandler(
	registerUC *application.RegisterProviderUseCase,
	listProvidersUC *application.ListProvidersUseCase,
) *ProviderHandler {
	return &ProviderHandler{
		registerUC:      registerUC,
		listProvidersUC: listProvidersUC,
	}
}

// POST /api/v1/providers
//
// Registro de proveedor técnico. Dos flujos según el rol del caller:
//   - PROVEEDOR: se autoregistra → user_id del body debe coincidir con el del JWT
//   - ADMIN_INMO: registra a un tercero → user_id del body es el del proveedor
//
// CallerUserID y CallerRoles se extraen del contexto inyectado por el JWT middleware,
// nunca del body — así el frontend no puede falsificar quién está llamando.
func (h *ProviderHandler) RegisterProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	// Leer claims del contexto (inyectados por jwtMiddleware en el router)
	callerUserID, _ := r.Context().Value(CtxKeyUserID).(string)
	callerRoles, _ := r.Context().Value(CtxKeyRoles).([]string)

	if callerUserID == "" {
		respondWithError(w, http.StatusUnauthorized, "Token de autenticación requerido")
		return
	}

	var body struct {
		UserID              string              `json:"user_id"`
		RazonSocial         string              `json:"razon_social"`
		CuitCuil            string              `json:"cuit_cuil"`
		Rubro               domain.RubroTecnico `json:"rubro"`
		DisponibleUrgencias bool                `json:"disponible_urgencias"`
		CbuPago             string              `json:"cbu_pago"`
		AliasPago           string              `json:"alias_pago"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	// Si el proveedor no envió su propio user_id, lo tomamos del JWT (autoregistro)
	targetUserID := body.UserID
	if targetUserID == "" {
		targetUserID = callerUserID
	}

	cmd := application.RegisterProviderCommand{
		TargetUserID:        targetUserID,
		RazonSocial:         body.RazonSocial,
		CuitCuil:            body.CuitCuil,
		Rubro:               body.Rubro,
		DisponibleUrgencias: body.DisponibleUrgencias,
		CbuPago:             body.CbuPago,
		AliasPago:           body.AliasPago,
		CallerUserID:        callerUserID,
		CallerRoles:         callerRoles,
	}

	resp, err := h.registerUC.Execute(r.Context(), cmd)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrProviderAlreadyRegistered):
			respondWithError(w, http.StatusConflict, err.Error())
		case errors.Is(err, application.ErrCuitCuilAlreadyExists):
			respondWithError(w, http.StatusConflict, err.Error())
		case errors.Is(err, application.ErrUnauthorizedRegistration):
			respondWithError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, domain.ErrInvalidRubro):
			respondWithError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrInvalidCuitCuil):
			respondWithError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrRazonSocialEmpty):
			respondWithError(w, http.StatusBadRequest, err.Error())
		default:
			respondWithError(w, http.StatusInternalServerError, "Error interno del servidor")
		}
		return
	}

	respondWithJSON(w, http.StatusCreated, resp)
}

// GET /api/v1/providers/search?rubro=PLOMERO&urgency=true
//
// Lista proveedores activos por rubro. Parámetro urgency=true filtra
// solo los que aceptan tickets EMERGENCY. Usado por ADMIN_INMO antes de asignar.
func (h *ProviderHandler) ListProvidersByRubro(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	rubroStr := r.URL.Query().Get("rubro")
	if rubroStr == "" {
		respondWithError(w, http.StatusBadRequest, "El parámetro 'rubro' es obligatorio")
		return
	}

	cmd := application.ListProvidersByRubroCommand{
		Rubro:       domain.RubroTecnico(rubroStr),
		OnlyUrgency: r.URL.Query().Get("urgency") == "true",
	}

	providers, err := h.listProvidersUC.Execute(r.Context(), cmd)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error al consultar proveedores")
		return
	}

	type providerDTO struct {
		ID                  string                `json:"id"`
		UserID              string                `json:"user_id"`
		RazonSocial         string                `json:"razon_social"`
		Rubro               domain.RubroTecnico   `json:"rubro"`
		DisponibleUrgencias bool                  `json:"disponible_urgencias"`
		Status              domain.ProviderStatus `json:"status"`
	}

	result := make([]providerDTO, 0, len(providers))
	for _, p := range providers {
		result = append(result, providerDTO{
			ID:                  p.ID(),
			UserID:              p.UserID(),
			RazonSocial:         p.RazonSocial(),
			Rubro:               p.Rubro(),
			DisponibleUrgencias: p.DisponibleUrgencias(),
			Status:              p.Status(),
		})
	}

	respondWithJSON(w, http.StatusOK, result)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, map[string]string{"error": msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
