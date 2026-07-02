package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestDeleteMedia_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := application.NewDeletePropertyMediaUseCase(repo, &fakeMediaRepo{})

	err := uc.Execute(context.Background(), application.DeleteMediaCommand{PropertyID: "prop-1", MediaID: "media-1", RequesterID: "owner-1"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestDeleteMedia_PropiedadNoExiste_RetornaNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := application.NewDeletePropertyMediaUseCase(repo, &fakeMediaRepo{})

	err := uc.Execute(context.Background(), application.DeleteMediaCommand{PropertyID: "no-existe", MediaID: "media-1", RequesterID: "owner-1"})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestDeleteMedia_NoEsElDueno_RetornaForbidden(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewDeletePropertyMediaUseCase(repo, &fakeMediaRepo{})

	err := uc.Execute(context.Background(), application.DeleteMediaCommand{PropertyID: "prop-1", MediaID: "media-1", RequesterID: "owner-intruso"})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("Execute: got %v, want AppError Forbidden", err)
	}
}

func TestDeleteMedia_ErrorAlBorrar_RetornaErrorSinEnvolver(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo de escritura")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{deleteErr: boom}
	uc := application.NewDeletePropertyMediaUseCase(repo, mediaRepo)

	err := uc.Execute(context.Background(), application.DeleteMediaCommand{PropertyID: "prop-1", MediaID: "media-1", RequesterID: "owner-1"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestDeleteMedia_HappyPath_BorraElMediaCorrecto(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	uc := application.NewDeletePropertyMediaUseCase(repo, mediaRepo)

	err := uc.Execute(context.Background(), application.DeleteMediaCommand{PropertyID: "prop-1", MediaID: "media-1", RequesterID: "owner-1"})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if mediaRepo.deletedID != "media-1" {
		t.Errorf("DeleteMedia: got mediaID=%q, want %q", mediaRepo.deletedID, "media-1")
	}
}
