package httpapi

import (
	"encoding/json"
	"net/http"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/shared/pkg/apperr"
)

type MediaHandler struct {
	generateUploadURL *application.GenerateUploadURLUseCase
	addMedia          *application.AddPropertyMediaUseCase
	listMedia         *application.ListPropertyMediaUseCase
}

func NewMediaHandler(
	generateUploadURL *application.GenerateUploadURLUseCase,
	addMedia *application.AddPropertyMediaUseCase,
	listMedia *application.ListPropertyMediaUseCase,
) *MediaHandler {
	return &MediaHandler{
		generateUploadURL: generateUploadURL,
		addMedia:          addMedia,
		listMedia:         listMedia,
	}
}

// HandleGenerateUploadURL: POST /api/v1/properties/{id}/media/upload-url
// Devuelve una presigned URL para subir directamente a S3.
func (h *MediaHandler) HandleGenerateUploadURL(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-Id")
	if ownerID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista por el gateway", nil))
		return
	}

	var req struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	resp, err := h.generateUploadURL.Execute(r.Context(), application.GenerateUploadURLCommand{
		PropertyID:  r.PathValue("id"),
		Filename:    req.Filename,
		ContentType: req.ContentType,
		RequesterID: ownerID,
	})
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleAddMedia: POST /api/v1/properties/{id}/media
// Guarda un registro de media después de que el front confirmó la subida a S3,
// o agrega directamente enlaces de redes sociales (type: SOCIAL_LINK).
func (h *MediaHandler) HandleAddMedia(w http.ResponseWriter, r *http.Request) {
	ownerID := r.Header.Get("X-User-Id")
	if ownerID == "" {
		h.errResp(w, apperr.NewBadRequest("identidad del usuario no provista por el gateway", nil))
		return
	}

	var req struct {
		URL         string            `json:"url"`
		Type        string            `json:"type"`
		SortOrder   int               `json:"sort_order"`
		SocialLinks map[string]string `json:"social_links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errResp(w, apperr.NewBadRequest("JSON inválido", err))
		return
	}

	if err := h.addMedia.Execute(r.Context(), application.AddMediaCommand{
		PropertyID:  r.PathValue("id"),
		URL:         req.URL,
		Type:        req.Type,
		SortOrder:   req.SortOrder,
		SocialLinks: req.SocialLinks,
		RequesterID: ownerID,
	}); err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

// HandleListMedia: GET /api/v1/properties/{id}/media
func (h *MediaHandler) HandleListMedia(w http.ResponseWriter, r *http.Request) {
	items, err := h.listMedia.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.errResp(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(items)
}

func (h *MediaHandler) errResp(w http.ResponseWriter, err error) {
	statusCode := apperr.HTTPStatusCode(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if appErr, ok := err.(*apperr.AppError); ok {
		_ = json.NewEncoder(w).Encode(appErr)
		return
	}
	_, _ = w.Write([]byte(`{"type":"INTERNAL_SERVER_ERROR","message":"error inesperado"}`))
}
