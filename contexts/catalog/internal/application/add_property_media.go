package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

type AddPropertyMediaUseCase struct {
	propertyRepo ports.PropertyRepository
	mediaRepo    ports.MediaRepository
	db           *sql.DB
	bucketName   string
	awsRegion    string
}

func NewAddPropertyMediaUseCase(propertyRepo ports.PropertyRepository, mediaRepo ports.MediaRepository, db *sql.DB) *AddPropertyMediaUseCase {
	bucketName := os.Getenv("AWS_BUCKET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}
	return &AddPropertyMediaUseCase{
		propertyRepo: propertyRepo,
		mediaRepo:    mediaRepo,
		db:           db,
		bucketName:   bucketName,
		awsRegion:    awsRegion,
	}
}

type AddMediaCommand struct {
	PropertyID  string
	URL         string
	Type        string
	SortOrder   int
	SocialLinks map[string]string
	RequesterID string
}

func (uc *AddPropertyMediaUseCase) Execute(ctx context.Context, cmd AddMediaCommand) error {
	property, err := uc.propertyRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return err
	}
	if property == nil {
		return apperr.NewNotFound("propiedad no encontrada", nil)
	}
	if property.OwnerID() != cmd.RequesterID {
		return apperr.NewForbidden("solo el dueño puede agregar media a esta propiedad", nil)
	}

	id := fmt.Sprintf("media-%d", time.Now().UnixNano())
	media, err := domain.NewPropertyMedia(id, cmd.PropertyID, cmd.URL, domain.MediaType(cmd.Type), cmd.SortOrder, cmd.SocialLinks)
	if err != nil {
		return err
	}

	if err := uc.mediaRepo.SaveMedia(ctx, media); err != nil {
		return err
	}

	// Solo publicar evento si es una imagen y hay configuración de S3
	if domain.MediaType(cmd.Type) == domain.MediaTypeImage && uc.bucketName != "" {
		event := domain.NewPropertyMediaAdded(media, cmd.RequesterID, uc.bucketName, uc.awsRegion)
		
		// Publicar mediante outbox para garantizar entrega
		if err := uc.publishViaOutbox(ctx, event); err != nil {
			// Log pero no fallar la operación principal
			fmt.Printf("[WARNING] No se pudo enqueue el evento PropertyMediaAdded: %v\n", err)
		}
	}

	return nil
}

// publishViaOutbox guarda el evento en la tabla outbox_events para procesamiento asíncrono
func (uc *AddPropertyMediaUseCase) publishViaOutbox(ctx context.Context, event ddd.DomainEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return apperr.NewInternal("error al serializar evento PropertyMediaAdded", err)
	}

	eventName := ""
	if baseEvent, ok := event.(interface{ EventName() string }); ok {
		eventName = baseEvent.EventName()
	}
	
	if eventName == "" {
		return apperr.NewInternal("evento sin nombre válido", nil)
	}

	query := `
		INSERT INTO outbox_events (id, event_name, payload, status, created_at)
		VALUES ($1, $2, $3, 'PENDING', CURRENT_TIMESTAMP)
	`

	_, err = uc.db.ExecContext(ctx, query, nextUUID(), eventName, payload)
	if err != nil {
		return apperr.NewInternal("no se pudo guardar evento en outbox", err)
	}
	return nil
}

// nextUUID genera un UUID simple para el evento
func nextUUID() string {
	b := make([]byte, 16)
	_, _ = fmt.Fprintf(b, "%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return string(b)
}
