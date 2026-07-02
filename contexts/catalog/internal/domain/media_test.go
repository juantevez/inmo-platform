package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── NewPropertyMedia ───────────────────────────────────────────────────────

func TestNewPropertyMedia_IDVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPropertyMedia("", "prop-1", "https://x.test/a.jpg", domain.MediaTypeImage, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPropertyMedia: got %v, want AppError BadRequest", err)
	}
}

func TestNewPropertyMedia_PropertyIDVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPropertyMedia("media-1", "", "https://x.test/a.jpg", domain.MediaTypeImage, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPropertyMedia: got %v, want AppError BadRequest", err)
	}
}

func TestNewPropertyMedia_TipoInvalido_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPropertyMedia("media-1", "prop-1", "https://x.test/a.pdf", domain.MediaType("PDF"), 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPropertyMedia: got %v, want AppError BadRequest", err)
	}
}

func TestNewPropertyMedia_Imagen_SinURL_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPropertyMedia("media-1", "prop-1", "", domain.MediaTypeImage, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPropertyMedia: got %v, want AppError BadRequest (URL obligatoria para IMAGE)", err)
	}
}

func TestNewPropertyMedia_Video_SinURL_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPropertyMedia("media-1", "prop-1", "", domain.MediaTypeVideo, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPropertyMedia: got %v, want AppError BadRequest (URL obligatoria para VIDEO)", err)
	}
}

func TestNewPropertyMedia_SocialLink_SinLinks_RetornaBadRequest(t *testing.T) {
	cases := []struct {
		name  string
		links map[string]string
	}{
		{"nil", nil},
		{"mapa vacío", map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewPropertyMedia("media-1", "prop-1", "", domain.MediaTypeSocialLink, 0, tc.links)

			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
				t.Fatalf("NewPropertyMedia(links=%v): got %v, want AppError BadRequest", tc.links, err)
			}
		})
	}
}

func TestNewPropertyMedia_SocialLink_NoRequiereURL(t *testing.T) {
	// A diferencia de IMAGE/VIDEO, SOCIAL_LINK no exige URL — solo social_links.
	links := map[string]string{"instagram": "https://instagram.com/depto"}
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "", domain.MediaTypeSocialLink, 0, links)

	if err != nil {
		t.Fatalf("NewPropertyMedia: error inesperado: %v", err)
	}
	if media.URL() != "" {
		t.Errorf("URL: got %q, want vacío", media.URL())
	}
	if media.SocialLinks()["instagram"] != "https://instagram.com/depto" {
		t.Errorf("SocialLinks: got %v", media.SocialLinks())
	}
}

func TestNewPropertyMedia_HappyPath_Imagen(t *testing.T) {
	before := time.Now()
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "https://x.test/a.jpg", domain.MediaTypeImage, 3, nil)
	after := time.Now()

	if err != nil {
		t.Fatalf("NewPropertyMedia: error inesperado: %v", err)
	}
	if media.ID() != "media-1" || media.PropertyID() != "prop-1" || media.URL() != "https://x.test/a.jpg" ||
		media.Type() != domain.MediaTypeImage || media.SortOrder() != 3 {
		t.Errorf("media: got %+v", media)
	}
	if media.CreatedAt().Before(before) || media.CreatedAt().After(after) {
		t.Errorf("CreatedAt: got %v, want entre %v y %v", media.CreatedAt(), before, after)
	}
	if media.UpdatedAt().Before(before) || media.UpdatedAt().After(after) {
		t.Errorf("UpdatedAt: got %v, want entre %v y %v", media.UpdatedAt(), before, after)
	}
}

func TestNewPropertyMedia_HappyPath_Video(t *testing.T) {
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "https://x.test/tour.mp4", domain.MediaTypeVideo, 0, nil)

	if err != nil {
		t.Fatalf("NewPropertyMedia: error inesperado: %v", err)
	}
	if media.Type() != domain.MediaTypeVideo {
		t.Errorf("Type: got %q, want %q", media.Type(), domain.MediaTypeVideo)
	}
}

// ─── ReconstructPropertyMedia ───────────────────────────────────────────────

func TestReconstructPropertyMedia_BypaseaValidacionesYPreservaTimestamps(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 6, 1, 15, 30, 0, 0, time.UTC)

	// Reconstruct no valida nada — permite incluso datos que NewPropertyMedia rechazaría,
	// como corresponde a la hidratación desde infraestructura (ej: Postgres).
	media := domain.ReconstructPropertyMedia("media-1", "prop-1", "", domain.MediaType("LEGACY_TYPE"), 5, nil, createdAt, updatedAt)

	if media.ID() != "media-1" || media.PropertyID() != "prop-1" || media.SortOrder() != 5 {
		t.Errorf("media: got %+v", media)
	}
	if media.Type() != domain.MediaType("LEGACY_TYPE") {
		t.Errorf("Type: got %q, want se preserve tal cual (sin validar)", media.Type())
	}
	if !media.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt: got %v, want %v", media.CreatedAt(), createdAt)
	}
	if !media.UpdatedAt().Equal(updatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", media.UpdatedAt(), updatedAt)
	}
}
