package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"inmo.platform/contexts/auth-identity/internal/application"
)

type AuthHandler struct {
	registerUC    *application.RegisterUserUseCase
	loginPassUC   *application.LoginPasswordUseCase
	verifyEmailUC *application.VerifyEmailUseCase
	loginGoogleUC *application.LoginSSOGoogleUseCase
}

func NewAuthHandler(
	registerUC *application.RegisterUserUseCase,
	loginPassUC *application.LoginPasswordUseCase,
	verifyEmailUC *application.VerifyEmailUseCase,
	loginGoogleUC *application.LoginSSOGoogleUseCase,
) *AuthHandler {
	return &AuthHandler{
		registerUC:    registerUC,
		loginPassUC:   loginPassUC,
		verifyEmailUC: verifyEmailUC,
		loginGoogleUC: loginGoogleUC,
	}
}

// HandleRegister maneja el endpoint POST /auth/register (UC-01)
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Payload JSON inválido")
		return
	}

	cmd := application.RegisterUserCommand{
		Email:    req.Email,
		Password: req.Password,
	}

	resp, err := h.registerUC.Execute(r.Context(), cmd)
	if err != nil {
		// Mapeo selectivo de errores de negocio a HTTP Status Codes
		if errors.Is(err, application.ErrEmailAlreadyExists) {
			h.respondWithError(w, http.StatusConflict, err.Error()) // 409
			return
		}
		h.respondWithError(w, http.StatusUnprocessableEntity, err.Error()) // 422
		return
	}

	h.respondWithJSON(w, http.StatusCreated, resp)
}

// HandleLoginPassword maneja el endpoint POST /auth/login (UC-03)
func (h *AuthHandler) HandleLoginPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Payload JSON inválido")
		return
	}

	// Extraemos IP del cliente y User-Agent para el logueo de seguridad de tu requerimiento
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}

	cmd := application.LoginPasswordCommand{
		Email:     req.Email,
		Password:  req.Password,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
	}

	resp, err := h.loginPassUC.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrRateLimitExceeded) {
			h.respondWithError(w, http.StatusTooManyRequests, err.Error()) // 429
			return
		}
		if errors.Is(err, application.ErrInvalidCredentials) {
			h.respondWithError(w, http.StatusUnauthorized, err.Error()) // 401
			return
		}
		if errors.Is(err, application.ErrEmailNotVerified) {
			h.respondWithError(w, http.StatusForbidden, err.Error()) // 403
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	h.respondWithJSON(w, http.StatusOK, resp)
}

// HandleVerifyEmail maneja el endpoint GET /auth/verify (UC-02)
func (h *AuthHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	tokenValue := r.URL.Query().Get("token")
	if tokenValue == "" {
		h.respondWithError(w, http.StatusBadRequest, "El parámetro 'token' es obligatorio")
		return
	}

	cmd := application.VerifyEmailCommand{TokenValue: tokenValue}
	resp, err := h.verifyEmailUC.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrTokenNotFound) {
			h.respondWithError(w, http.StatusNotFound, err.Error()) // 404
			return
		}
		h.respondWithError(w, http.StatusBadRequest, err.Error()) // 400
		return
	}

	h.respondWithJSON(w, http.StatusOK, resp)
}

// Helpers globales de respuesta
func (h *AuthHandler) respondWithError(w http.ResponseWriter, code int, msg string) {
	h.respondWithJSON(w, code, map[string]string{"error": msg})
}

func (h *AuthHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
