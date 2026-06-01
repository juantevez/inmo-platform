package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
)

// ProfileJSONRequest mapea el cuerpo que envía el Frontend en formato JSON
type ProfileJSONRequest struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	DniCuit       string `json:"dni_cuit"`
	Phone         string `json:"phone"`
	ProfileType   string `json:"profile_type"`   // "INDIVIDUAL" o "COMMERCIAL"
	CompanyName   string `json:"company_name"`   // Opcional
	LicenseNumber string `json:"license_number"` // Opcional
}

// ProfileHandler orquesta las peticiones HTTP hacia el caso de uso
type ProfileHandler struct {
	createProfileUC *application.CreateProfileUseCase
}

func NewProfileHandler(createProfileUC *application.CreateProfileUseCase) *ProfileHandler {
	return &ProfileHandler{createProfileUC: createProfileUC}
}

// HandleCreateOrUpdate gestiona el endpoint POST /api/v1/catalog/profiles
func (h *ProfileHandler) HandleCreateOrUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	// 1. Capturar el User ID inyectado por el API Gateway desde el Header
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		// Si está vacío, significa que alguien se salteó el Gateway o está mal configurado
		h.respondWithError(w, http.StatusUnauthorized, "Identidad del usuario no provista por el Gateway")
		return
	}

	// 2. Decodificar el JSON de entrada
	var req ProfileJSONRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Cuerpo de la petición JSON inválido")
		return
	}

	// 3. Mapear el DTO de HTTP al Command de Aplicación
	cmd := application.CreateProfileCommand{
		UserID:        userID, // ◄ Atado de forma segura a su sesión
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		DniCuit:       req.DniCuit,
		Phone:         req.Phone,
		ProfileType:   req.ProfileType,
		CompanyName:   req.CompanyName,
		LicenseNumber: req.LicenseNumber,
	}

	// 4. Ejecutar el caso de uso y atrapar errores específicos
	err := h.createProfileUC.Execute(r.Context(), cmd)
	if err != nil {
		h.mapUseCaseErrorToHTTP(w, err)
		return
	}

	// 5. Responder éxito (201 Created)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Perfil de negocio registrado o actualizado correctamente",
		"status":  "PENDING_VERIFICATION",
	})
}

// Helper para centralizar las respuestas de error estructuradas
func (h *ProfileHandler) respondWithError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Mapper de errores: Traduce errores de Negocio/Dominio a códigos de estado HTTP correctos
func (h *ProfileHandler) mapUseCaseErrorToHTTP(w http.ResponseWriter, err error) {
	switch {
	// Errores de Validación de Dominio (400 Bad Request)
	case errors.Is(err, domain.ErrMissingRequiredFields),
		errors.Is(err, domain.ErrInvalidProfileType),
		errors.Is(err, domain.ErrCommercialMissingData):
		h.respondWithError(w, http.StatusBadRequest, err.Error())

	// Conflictos de negocio (409 Conflict)
	case errors.Is(err, application.ErrDniCuitAlreadyExists):
		h.respondWithError(w, http.StatusConflict, err.Error())

	// Fallos de infraestructura ocultados por seguridad (500 Internal Server Error)
	default:
		// Acá idealmente loggeás el error real en tu consola de Docker para debuggear
		h.respondWithError(w, http.StatusInternalServerError, "Ocurrió un error interno en el servidor")
	}
}
