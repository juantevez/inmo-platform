package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"inmo.platform/contexts/auth-identity/internal/application"
)

type SSOPublicConfig struct {
	GoogleClientID    string `json:"google_client_id"`
	GoogleRedirectURI string `json:"google_redirect_uri"`
	MetaAppID         string `json:"meta_app_id"`
}

type AuthHandler struct {
	registerUC    *application.RegisterUserUseCase
	loginPassUC   *application.LoginPasswordUseCase
	verifyEmailUC *application.VerifyEmailUseCase
	loginGoogleUC *application.LoginSSOGoogleUseCase
	loginMetaUC   *application.LoginSSOMetaUseCase
	ssoConfig     SSOPublicConfig
}

func NewAuthHandler(
	registerUC *application.RegisterUserUseCase,
	loginPassUC *application.LoginPasswordUseCase,
	verifyEmailUC *application.VerifyEmailUseCase,
	loginGoogleUC *application.LoginSSOGoogleUseCase,
	loginMetaUC *application.LoginSSOMetaUseCase,
	ssoConfig SSOPublicConfig,
) *AuthHandler {
	return &AuthHandler{
		registerUC:    registerUC,
		loginPassUC:   loginPassUC,
		verifyEmailUC: verifyEmailUC,
		loginGoogleUC: loginGoogleUC,
		loginMetaUC:   loginMetaUC,
		ssoConfig:     ssoConfig,
	}
}

// HandleRegister maneja el endpoint POST /auth/register (UC-01)
// El frontend DEBE enviar el campo "role" con uno de los valores válidos:
// INQUILINO | PROPIETARIO | AGENTE | PROVEEDOR | INTERESADO
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"` // ← NUEVO: requerido desde el frontend
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Payload JSON inválido")
		return
	}

	cmd := application.RegisterUserCommand{
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role, // ← se propaga al use case
	}

	resp, err := h.registerUC.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrEmailAlreadyExists) {
			h.respondWithError(w, http.StatusConflict, err.Error()) // 409
			return
		}
		if errors.Is(err, application.ErrInvalidRole) {
			h.respondWithError(w, http.StatusBadRequest, err.Error()) // 400
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

	clientIP := h.extractClientIP(r)

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

// HandleGoogleLogin maneja el endpoint POST /auth/sso/google (UC-04)
func (h *AuthHandler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Payload JSON inválido. Se requiere 'code'")
		return
	}

	cmd := application.LoginSSOGoogleCommand{
		Code:      req.Code,
		ClientIP:  h.extractClientIP(r),
		UserAgent: r.UserAgent(),
	}

	resp, err := h.loginGoogleUC.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrLinkVerificationRequired) {
			h.respondWithError(w, http.StatusConflict, err.Error())
			return
		}
		h.respondWithError(w, http.StatusUnprocessableEntity, err.Error()) // 422
		return
	}

	h.respondWithJSON(w, http.StatusOK, resp)
}

// HandleMetaLogin maneja el endpoint POST /auth/sso/meta (UC-05)
func (h *AuthHandler) HandleMetaLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Payload JSON inválido. Se requiere 'access_token'")
		return
	}

	cmd := application.LoginSSOMetaCommand{
		AccessToken: req.AccessToken,
		ClientIP:    h.extractClientIP(r),
		UserAgent:   r.UserAgent(),
	}

	resp, err := h.loginMetaUC.Execute(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, application.ErrLinkVerificationRequired) {
			h.respondWithError(w, http.StatusConflict, err.Error())
			return
		}
		h.respondWithError(w, http.StatusUnprocessableEntity, err.Error()) // 422
		return
	}

	h.respondWithJSON(w, http.StatusOK, resp)
}

// HandleSSOConfig: GET /api/v1/auth/sso/config
func (h *AuthHandler) HandleSSOConfig(w http.ResponseWriter, r *http.Request) {
	h.respondWithJSON(w, http.StatusOK, h.ssoConfig)
}

func (h *AuthHandler) extractClientIP(r *http.Request) string {
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}
	return clientIP
}

func (h *AuthHandler) respondWithError(w http.ResponseWriter, code int, msg string) {
	h.respondWithJSON(w, code, map[string]string{"error": msg})
}

func (h *AuthHandler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
