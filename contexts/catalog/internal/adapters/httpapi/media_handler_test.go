package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeMediaRepo struct {
	items      []*domain.PropertyMedia
	saveErr    error
	findErr    error
	deleteErr  error
	deletedID  string
	savedMedia *domain.PropertyMedia
}

func (f *fakeMediaRepo) SaveMedia(ctx context.Context, media *domain.PropertyMedia) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedMedia = media
	return nil
}
func (f *fakeMediaRepo) FindByPropertyID(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.items, nil
}
func (f *fakeMediaRepo) DeleteMedia(ctx context.Context, mediaID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedID = mediaID
	return nil
}

type fakeMediaStorage struct {
	presignedURL string
	finalURL     string
	err          error
}

func (f *fakeMediaStorage) GeneratePresignedURL(ctx context.Context, propertyID, filename, contentType string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.presignedURL, f.finalURL, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newMediaHandler(t *testing.T, repo ports.PropertyRepository, mediaRepo ports.MediaRepository, storage ports.MediaStorageProvider) *httpapi.MediaHandler {
	t.Helper()
	// AWS_BUCKET_NAME debe quedar vacío para que AddPropertyMediaUseCase no intente
	// escribir en el outbox (que requiere un *sql.DB real) durante estos tests.
	t.Setenv("AWS_BUCKET_NAME", "")

	generateUploadURL := application.NewGenerateUploadURLUseCase(repo, storage)
	addMedia := application.NewAddPropertyMediaUseCase(repo, mediaRepo, nil)
	listMedia := application.NewListPropertyMediaUseCase(mediaRepo)
	deleteMedia := application.NewDeletePropertyMediaUseCase(repo, mediaRepo)

	return httpapi.NewMediaHandler(generateUploadURL, addMedia, listMedia, deleteMedia)
}

// ─── HandleGenerateUploadURL ────────────────────────────────────────────────

func TestHandleGenerateUploadURL_SinXUserId_Retorna400(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media/upload-url", bytes.NewBufferString("{}"))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGenerateUploadURL_JSONInvalido_Retorna400(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media/upload-url", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGenerateUploadURL_StorageNoConfigurado_Retorna400(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, nil) // storage nil == AWS no configurado

	body, _ := json.Marshal(map[string]string{"filename": "foto.jpg", "content_type": "image/jpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media/upload-url", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleGenerateUploadURL_PropiedadNoExiste_Retorna404(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	body, _ := json.Marshal(map[string]string{"filename": "foto.jpg", "content_type": "image/jpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/media/upload-url", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGenerateUploadURL_NoEsElDueno_Retorna403(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1") // owner-1
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, &fakeMediaStorage{})

	body, _ := json.Marshal(map[string]string{"filename": "foto.jpg", "content_type": "image/jpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media/upload-url", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-intruso")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleGenerateUploadURL_HappyPath_Retorna200ConURLs(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1") // owner-1
	storage := &fakeMediaStorage{presignedURL: "https://s3.test/presigned", finalURL: "https://cdn.test/final.jpg"}
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, storage)

	body, _ := json.Marshal(map[string]string{"filename": "foto.jpg", "content_type": "image/jpeg"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media/upload-url", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleGenerateUploadURL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp application.UploadURLResponse
	decodeBody(t, rec, &resp)
	if resp.PresignedURL != "https://s3.test/presigned" || resp.FinalURL != "https://cdn.test/final.jpg" {
		t.Errorf("UploadURLResponse: got %+v", resp)
	}
}

// ─── HandleAddMedia ─────────────────────────────────────────────────────────

func TestHandleAddMedia_SinXUserId_Retorna400(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBufferString("{}"))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAddMedia_JSONInvalido_Retorna400(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAddMedia_PropiedadNoExiste_Retorna404(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	body, _ := json.Marshal(map[string]interface{}{"url": "https://x.test/a.jpg", "type": "IMAGE"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/media", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAddMedia_NoEsElDueno_Retorna403(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1") // owner-1
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, &fakeMediaStorage{})

	body, _ := json.Marshal(map[string]interface{}{"url": "https://x.test/a.jpg", "type": "IMAGE"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-intruso")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleAddMedia_TipoDeMediaInvalido_Retorna400(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, &fakeMediaStorage{})

	body, _ := json.Marshal(map[string]interface{}{"url": "https://x.test/a.jpg", "type": "PDF"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleAddMedia_HappyPath_SocialLink_Retorna201(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	mediaRepo := &fakeMediaRepo{}
	h := newMediaHandler(t, repo, mediaRepo, &fakeMediaStorage{})

	body, _ := json.Marshal(map[string]interface{}{
		"type":         "SOCIAL_LINK",
		"social_links": map[string]string{"instagram": "https://instagram.com/depto"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if mediaRepo.savedMedia == nil || mediaRepo.savedMedia.Type() != domain.MediaTypeSocialLink {
		t.Errorf("SaveMedia: got %+v, want un media SOCIAL_LINK persistido", mediaRepo.savedMedia)
	}
}

func TestHandleAddMedia_ErrorAlGuardar_Retorna500(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	mediaRepo := &fakeMediaRepo{saveErr: errors.New("fallo de escritura")}
	h := newMediaHandler(t, repo, mediaRepo, &fakeMediaStorage{})

	body, _ := json.Marshal(map[string]interface{}{"url": "https://x.test/a.jpg", "type": "IMAGE"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/media", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleAddMedia(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── HandleListMedia ────────────────────────────────────────────────────────

func TestHandleListMedia_HappyPath_Retorna200ConLista(t *testing.T) {
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "https://x.test/a.jpg", domain.MediaTypeImage, 0, nil)
	if err != nil {
		t.Fatalf("NewPropertyMedia: %v", err)
	}
	mediaRepo := &fakeMediaRepo{items: []*domain.PropertyMedia{media}}
	h := newMediaHandler(t, &fakeRepo{}, mediaRepo, &fakeMediaStorage{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties/prop-1/media", nil)
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleListMedia(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var got []application.MediaDTO
	decodeBody(t, rec, &got)
	if len(got) != 1 || got[0].ID != "media-1" {
		t.Errorf("MediaDTO list: got %+v", got)
	}
}

func TestHandleListMedia_Error_Retorna500(t *testing.T) {
	mediaRepo := &fakeMediaRepo{findErr: errors.New("timeout de base de datos")}
	h := newMediaHandler(t, &fakeRepo{}, mediaRepo, &fakeMediaStorage{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties/prop-1/media", nil)
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.HandleListMedia(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── HandleDeleteMedia ──────────────────────────────────────────────────────

func TestHandleDeleteMedia_SinXUserId_Retorna400(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/properties/prop-1/media/media-1", nil)
	req.SetPathValue("id", "prop-1")
	req.SetPathValue("mediaID", "media-1")
	rec := httptest.NewRecorder()

	h.HandleDeleteMedia(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteMedia_PropiedadNoExiste_Retorna404(t *testing.T) {
	h := newMediaHandler(t, &fakeRepo{}, &fakeMediaRepo{}, &fakeMediaStorage{})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/properties/no-existe/media/media-1", nil)
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "no-existe")
	req.SetPathValue("mediaID", "media-1")
	rec := httptest.NewRecorder()

	h.HandleDeleteMedia(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteMedia_NoEsElDueno_Retorna403(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1") // owner-1
	h := newMediaHandler(t, repo, &fakeMediaRepo{}, &fakeMediaStorage{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/properties/prop-1/media/media-1", nil)
	req.Header.Set("X-User-Id", "owner-intruso")
	req.SetPathValue("id", "prop-1")
	req.SetPathValue("mediaID", "media-1")
	rec := httptest.NewRecorder()

	h.HandleDeleteMedia(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleDeleteMedia_HappyPath_Retorna204(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1") // owner-1
	mediaRepo := &fakeMediaRepo{}
	h := newMediaHandler(t, repo, mediaRepo, &fakeMediaStorage{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/properties/prop-1/media/media-1", nil)
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	req.SetPathValue("mediaID", "media-1")
	rec := httptest.NewRecorder()

	h.HandleDeleteMedia(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNoContent)
	}
	if mediaRepo.deletedID != "media-1" {
		t.Errorf("DeleteMedia: got mediaID=%q, want %q", mediaRepo.deletedID, "media-1")
	}
}
