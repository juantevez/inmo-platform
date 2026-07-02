package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestListPropertyMedia_ErrorDelRepositorio_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	mediaRepo := &fakeMediaRepo{
		findByPropIDFn: func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) { return nil, boom },
	}
	uc := application.NewListPropertyMediaUseCase(mediaRepo)

	_, err := uc.Execute(context.Background(), "prop-1")

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestListPropertyMedia_SinResultados_RetornaListaVacia(t *testing.T) {
	mediaRepo := &fakeMediaRepo{
		findByPropIDFn: func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) { return nil, nil },
	}
	uc := application.NewListPropertyMediaUseCase(mediaRepo)

	dtos, err := uc.Execute(context.Background(), "prop-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dtos == nil {
		t.Error("Execute: no debería devolver nil (se inicializa con make([]MediaDTO, 0, ...))")
	}
	if len(dtos) != 0 {
		t.Errorf("Execute: got %v, want lista vacía", dtos)
	}
}

func TestListPropertyMedia_HappyPath_MapeaTodosLosCampos(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	media := domain.ReconstructPropertyMedia("media-1", "prop-1", "https://x.test/a.jpg", domain.MediaTypeImage, 2, nil, createdAt, createdAt)
	mediaRepo := &fakeMediaRepo{
		findByPropIDFn: func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
			if propertyID != "prop-1" {
				t.Errorf("FindByPropertyID: got %q, want %q", propertyID, "prop-1")
			}
			return []*domain.PropertyMedia{media}, nil
		},
	}
	uc := application.NewListPropertyMediaUseCase(mediaRepo)

	dtos, err := uc.Execute(context.Background(), "prop-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("Execute: got %d items, want 1", len(dtos))
	}
	dto := dtos[0]
	if dto.ID != "media-1" || dto.PropertyID != "prop-1" || dto.URL != "https://x.test/a.jpg" ||
		dto.Type != string(domain.MediaTypeImage) || dto.SortOrder != 2 || !dto.CreatedAt.Equal(createdAt) {
		t.Errorf("MediaDTO: got %+v", dto)
	}
}

func TestListPropertyMedia_SocialLink_PropagaLosLinks(t *testing.T) {
	links := map[string]string{"instagram": "https://instagram.com/depto"}
	media := domain.ReconstructPropertyMedia("media-1", "prop-1", "", domain.MediaTypeSocialLink, 0, links, time.Now(), time.Now())
	mediaRepo := &fakeMediaRepo{
		findByPropIDFn: func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
			return []*domain.PropertyMedia{media}, nil
		},
	}
	uc := application.NewListPropertyMediaUseCase(mediaRepo)

	dtos, err := uc.Execute(context.Background(), "prop-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dtos[0].SocialLinks["instagram"] != "https://instagram.com/depto" {
		t.Errorf("MediaDTO.SocialLinks: got %v", dtos[0].SocialLinks)
	}
}

func TestListPropertyMedia_MultiplesItems_PreservaElOrden(t *testing.T) {
	m1 := domain.ReconstructPropertyMedia("media-1", "prop-1", "https://x.test/1.jpg", domain.MediaTypeImage, 0, nil, time.Now(), time.Now())
	m2 := domain.ReconstructPropertyMedia("media-2", "prop-1", "https://x.test/2.jpg", domain.MediaTypeImage, 1, nil, time.Now(), time.Now())
	mediaRepo := &fakeMediaRepo{
		findByPropIDFn: func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
			return []*domain.PropertyMedia{m1, m2}, nil
		},
	}
	uc := application.NewListPropertyMediaUseCase(mediaRepo)

	dtos, err := uc.Execute(context.Background(), "prop-1")

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if len(dtos) != 2 || dtos[0].ID != "media-1" || dtos[1].ID != "media-2" {
		t.Errorf("Execute: got %+v, want [media-1, media-2] en orden", dtos)
	}
}
