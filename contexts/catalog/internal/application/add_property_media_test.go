package application_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeMediaRepo struct {
	saveErr        error
	savedMedia     *domain.PropertyMedia
	deleteErr      error
	deletedID      string
	findByPropIDFn func(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error)
}

func (f *fakeMediaRepo) SaveMedia(ctx context.Context, media *domain.PropertyMedia) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedMedia = media
	return nil
}
func (f *fakeMediaRepo) FindByPropertyID(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
	if f.findByPropIDFn != nil {
		return f.findByPropIDFn(ctx, propertyID)
	}
	return nil, errors.New("FindByPropertyID: fixture no configurada")
}
func (f *fakeMediaRepo) DeleteMedia(ctx context.Context, mediaID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedID = mediaID
	return nil
}

// payloadContains matchea un argumento []byte de sqlmock que contenga cierto substring —
// se usa para verificar el contenido del evento serializado sin depender de sus campos
// no determinísticos (EventID, timestamp, MediaID con nanotime).
type payloadContains struct{ substr string }

func (p payloadContains) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	if !ok {
		return false
	}
	return strings.Contains(string(b), p.substr)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestAddMedia_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := application.NewAddPropertyMediaUseCase(repo, &fakeMediaRepo{}, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{PropertyID: "prop-1", RequesterID: "owner-1"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestAddMedia_PropiedadNoExiste_RetornaNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := application.NewAddPropertyMediaUseCase(repo, &fakeMediaRepo{}, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{PropertyID: "no-existe", RequesterID: "owner-1"})

	if !strings.Contains(err.Error(), "propiedad no encontrada") {
		t.Fatalf("Execute: got %v, want error de propiedad no encontrada", err)
	}
}

func TestAddMedia_NoEsElDueno_RetornaForbidden(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewAddPropertyMediaUseCase(repo, &fakeMediaRepo{}, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{PropertyID: "prop-1", RequesterID: "owner-intruso"})

	if !strings.Contains(err.Error(), "solo el dueño") {
		t.Fatalf("Execute: got %v, want error de forbidden", err)
	}
}

func TestAddMedia_TipoInvalido_RetornaErrorDeDominio(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := application.NewAddPropertyMediaUseCase(repo, &fakeMediaRepo{}, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1", URL: "https://x.test/a.pdf", Type: "PDF",
	})

	if err == nil || !strings.Contains(err.Error(), "tipo de media inválido") {
		t.Fatalf("Execute: got %v, want error de tipo de media inválido", err)
	}
}

func TestAddMedia_ErrorAlGuardarMedia_RetornaErrorSinEnvolver(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	boom := errors.New("fallo de escritura")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{saveErr: boom}
	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1", URL: "https://x.test/a.jpg", Type: "IMAGE",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestAddMedia_HappyPath_SocialLink_NoTocaLaDB(t *testing.T) {
	// SOCIAL_LINK nunca dispara el outbox (esa rama exige MediaTypeImage), así que
	// pasar db=nil es seguro: si el código lo tocara, esto paniquearía y el test fallaría.
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, nil)

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1", Type: "SOCIAL_LINK",
		SocialLinks: map[string]string{"instagram": "https://instagram.com/depto"},
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if mediaRepo.savedMedia == nil || mediaRepo.savedMedia.Type() != domain.MediaTypeSocialLink {
		t.Fatalf("SaveMedia: got %+v, want un media SOCIAL_LINK", mediaRepo.savedMedia)
	}
	if mediaRepo.savedMedia.PropertyID() != "prop-1" {
		t.Errorf("SaveMedia PropertyID: got %q, want %q", mediaRepo.savedMedia.PropertyID(), "prop-1")
	}
}

func TestAddMedia_Imagen_SinBucketConfigurado_NoTocaLaDB(t *testing.T) {
	// AWS_BUCKET_NAME se lee en el constructor — sin configurar, la condición
	// "MediaTypeImage && bucketName != ''" es falsa y el outbox nunca se dispara.
	t.Setenv("AWS_BUCKET_NAME", "")
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, nil) // db nil: no debe tocarse

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1", URL: "https://bucket.s3.us-east-1.amazonaws.com/img.jpg", Type: "IMAGE",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if mediaRepo.savedMedia == nil {
		t.Fatal("SaveMedia: no fue invocado")
	}
}

func TestAddMedia_Video_ConBucketConfigurado_NoDisparaOutbox(t *testing.T) {
	// Solo MediaTypeImage dispara el outbox — VIDEO con bucket configurado no debe tocarlo.
	t.Setenv("AWS_BUCKET_NAME", "mi-bucket")
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, nil) // db nil: no debe tocarse

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1", URL: "https://bucket.s3.us-east-1.amazonaws.com/video.mp4", Type: "VIDEO",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
}

func TestAddMedia_Imagen_ConBucketConfigurado_EncolaEventoEnOutbox(t *testing.T) {
	t.Setenv("AWS_BUCKET_NAME", "mi-bucket")
	t.Setenv("AWS_REGION", "sa-east-1")
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO outbox_events`).
		WithArgs(sqlmock.AnyArg(), "catalog.property.media_added", payloadContains{substr: `"property_id":"prop-1"`}).
		WillReturnResult(sqlmock.NewResult(0, 1))

	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, db)

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1",
		URL: "https://bucket.s3.us-east-1.amazonaws.com/properties/prop-1/foto.jpg", Type: "IMAGE",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestAddMedia_ErrorEncolandoEnOutbox_NoFallaLaOperacionPrincipal(t *testing.T) {
	// El use case loggea el fallo del outbox pero deliberadamente no lo propaga:
	// la media ya quedó guardada y es lo que le importa al usuario.
	t.Setenv("AWS_BUCKET_NAME", "mi-bucket")
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	mediaRepo := &fakeMediaRepo{}
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO outbox_events`).WillReturnError(errors.New("fallo de escritura en outbox"))

	uc := application.NewAddPropertyMediaUseCase(repo, mediaRepo, db)

	err := uc.Execute(context.Background(), application.AddMediaCommand{
		PropertyID: "prop-1", RequesterID: "owner-1",
		URL: "https://bucket.s3.us-east-1.amazonaws.com/img.jpg", Type: "IMAGE",
	})

	if err != nil {
		t.Fatalf("Execute: got %v, want nil (el fallo del outbox no debe propagarse)", err)
	}
	if mediaRepo.savedMedia == nil {
		t.Error("SaveMedia: debería haberse guardado igual, pese al fallo del outbox")
	}
}
