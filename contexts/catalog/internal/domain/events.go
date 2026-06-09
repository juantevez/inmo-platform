package domain

import (
	"crypto/rand"
	"fmt"
	"inmo.platform/shared/pkg/ddd"
)

// PropertySnapshot contiene la copia mínima de datos que Contratos necesita para
// calcular precios y validar restricciones de reserva sin llamar a Catálogo en tiempo real.
type PropertySnapshot struct {
	OwnerID         string        `json:"owner_id"`
	OperationType   string        `json:"operation_type"`
	NightPrice      float64       `json:"night_price"`
	CleaningFee     float64       `json:"cleaning_fee"`
	SecurityDeposit float64       `json:"security_deposit"`
	MinNights       int           `json:"min_nights"`
	MaxNights       int           `json:"max_nights"`
	CheckInTime     string        `json:"check_in_time"`
	CheckOutTime    string        `json:"check_out_time"`
	PricingRules    []PricingRule `json:"pricing_rules"`
}

// PropertyPublished se dispara cuando una propiedad pasa a estar disponible en el catálogo.
type PropertyPublished struct {
	ddd.BaseDomainEvent
	OwnerID  string          `json:"owner_id"`
	Snapshot PropertySnapshot `json:"snapshot"`
}

func NewPropertyPublished(p *Property) PropertyPublished {
	tc := p.TempConfig()
	return PropertyPublished{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			p.ID(),
			"catalog.property.published",
		),
		OwnerID: p.OwnerID(),
		Snapshot: PropertySnapshot{
			OwnerID:         p.OwnerID(),
			OperationType:   string(p.OperationType()),
			NightPrice:      tc.NightPrice(),
			CleaningFee:     tc.CleaningFee(),
			SecurityDeposit: tc.SecurityDeposit(),
			MinNights:       tc.MinNights(),
			MaxNights:       tc.MaxNights(),
			CheckInTime:     tc.CheckInTime(),
			CheckOutTime:    tc.CheckOutTime(),
			PricingRules:    tc.PricingRules(),
		},
	}
}

// PropertyUpdated se dispara cuando el propietario modifica precio/amenities de la propiedad.
type PropertyUpdated struct {
	ddd.BaseDomainEvent
	Snapshot PropertySnapshot `json:"snapshot"`
}

func NewPropertyUpdated(p *Property) PropertyUpdated {
	tc := p.TempConfig()
	return PropertyUpdated{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			p.ID(),
			"catalog.property.updated",
		),
		Snapshot: PropertySnapshot{
			OwnerID:         p.OwnerID(),
			OperationType:   string(p.OperationType()),
			NightPrice:      tc.NightPrice(),
			CleaningFee:     tc.CleaningFee(),
			SecurityDeposit: tc.SecurityDeposit(),
			MinNights:       tc.MinNights(),
			MaxNights:       tc.MaxNights(),
			CheckInTime:     tc.CheckInTime(),
			CheckOutTime:    tc.CheckOutTime(),
			PricingRules:    tc.PricingRules(),
		},
	}
}

// PropertyStateChanged se dispara ante cualquier transición en la máquina de estados.
type PropertyStateChanged struct {
	ddd.BaseDomainEvent
	OldState PropertyState `json:"old_state"`
	NewState PropertyState `json:"new_state"`
}

func NewPropertyStateChanged(propertyID string, oldState, newState PropertyState) PropertyStateChanged {
	return PropertyStateChanged{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			propertyID,
			"catalog.property.state_changed",
		),
		OldState: oldState,
		NewState: newState,
	}
}

// PropertyMediaAdded se dispara cuando se agrega un nuevo archivo multimedia a una propiedad.
// Este evento es utilizado por el servicio de redimensionamiento de imágenes.
type PropertyMediaAdded struct {
	ddd.BaseDomainEvent
	MediaID    string   `json:"media_id"`
	PropertyID string   `json:"property_id"`
	URL        string   `json:"url"`
	MediaType  string   `json:"media_type"`
	OwnerID    string   `json:"owner_id"`
	BucketName string   `json:"bucket_name"`
	S3Key      string   `json:"s3_key"`
	Region     string   `json:"region"`
}

func NewPropertyMediaAdded(media *domain.PropertyMedia, ownerID, bucketName, region string) PropertyMediaAdded {
	s3Key := extractS3Key(media.URL())
	return PropertyMediaAdded{
		BaseDomainEvent: ddd.NewBaseDomainEvent(
			nextUUID(),
			media.PropertyID(),
			"catalog.property.media_added",
		),
		MediaID:    media.ID(),
		PropertyID: media.PropertyID(),
		URL:        media.URL(),
		MediaType:  string(media.Type()),
		OwnerID:    ownerID,
		BucketName: bucketName,
		S3Key:      s3Key,
		Region:     region,
	}
}

// extractS3Key extrae la clave S3 desde una URL pública de S3
// Ej: https://bucket.s3.us-east-1.amazonaws.com/properties/id/img.jpg -> properties/id/img.jpg
func extractS3Key(url string) string {
	if len(url) == 0 {
		return ""
	}
	// Buscar el patrón ".amazonaws.com/" y tomar todo lo que sigue
	const marker = ".amazonaws.com/"
	idx := findSubstring(url, marker)
	if idx == -1 {
		return ""
	}
	return url[idx+len(marker):]
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Helper rápido para generar IDs de eventos sin arrastrar dependencias externas pesadas aún
func nextUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
