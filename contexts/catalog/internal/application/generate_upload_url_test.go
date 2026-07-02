package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeMediaStorage struct {
	presignedURL string
	finalURL     string
	err          error

	calledPropertyID  string
	calledFilename    string
	calledContentType string
}

func (f *fakeMediaStorage) GeneratePresignedURL(ctx context.Context, propertyID, filename, contentType string) (string, string, error) {
	f.calledPropertyID = propertyID
	f.calledFilename = filename
	f.calledContentType = contentType
	if f.err != nil {
		return "", "", f.err
	}
	return f.presignedURL, f.finalURL, nil
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestGenerateUploadURL_StorageNoConfigurado_RetornaBadRequest(t *testing.T) {
	uc := application.NewGenerateUploadURLUseCase(&fakePropertyRepo{}, nil)

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "foto.jpg", RequesterID: "owner-1",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestGenerateUploadURL_PropertyIDVacio_RetornaBadRequest(t *testing.T) {
	uc := application.NewGenerateUploadURLUseCase(&fakePropertyRepo{}, &fakeMediaStorage{})

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "", Filename: "foto.jpg", RequesterID: "owner-1",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestGenerateUploadURL_FilenameVacio_RetornaBadRequest(t *testing.T) {
	uc := application.NewGenerateUploadURLUseCase(&fakePropertyRepo{}, &fakeMediaStorage{})

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "", RequesterID: "owner-1",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestGenerateUploadURL_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := application.NewGenerateUploadURLUseCase(repo, &fakeMediaStorage{})

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "foto.jpg", RequesterID: "owner-1",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestGenerateUploadURL_PropiedadNoExiste_RetornaNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := application.NewGenerateUploadURLUseCase(repo, &fakeMediaStorage{})

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "no-existe", Filename: "foto.jpg", RequesterID: "owner-1",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestGenerateUploadURL_NoEsElDueno_RetornaForbidden(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewGenerateUploadURLUseCase(repo, &fakeMediaStorage{})

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "foto.jpg", RequesterID: "owner-intruso",
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("Execute: got %v, want AppError Forbidden", err)
	}
}

func TestGenerateUploadURL_ErrorGenerandoURL_RetornaErrorSinEnvolver(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo de comunicación con S3")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	storage := &fakeMediaStorage{err: boom}
	uc := application.NewGenerateUploadURLUseCase(repo, storage)

	_, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "foto.jpg", RequesterID: "owner-1",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestGenerateUploadURL_HappyPath_RetornaURLsYPropagaParametros(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	storage := &fakeMediaStorage{presignedURL: "https://s3.test/presigned", finalURL: "https://cdn.test/final.jpg"}
	uc := application.NewGenerateUploadURLUseCase(repo, storage)

	resp, err := uc.Execute(context.Background(), application.GenerateUploadURLCommand{
		PropertyID: "prop-1", Filename: "foto.jpg", ContentType: "image/jpeg", RequesterID: "owner-1",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.PresignedURL != "https://s3.test/presigned" || resp.FinalURL != "https://cdn.test/final.jpg" {
		t.Errorf("UploadURLResponse: got %+v", resp)
	}
	if storage.calledPropertyID != "prop-1" || storage.calledFilename != "foto.jpg" || storage.calledContentType != "image/jpeg" {
		t.Errorf("GeneratePresignedURL args: got propertyID=%q filename=%q contentType=%q",
			storage.calledPropertyID, storage.calledFilename, storage.calledContentType)
	}
}
