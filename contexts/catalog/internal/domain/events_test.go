package domain_test

import (
	"regexp"
	"testing"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// uuidPattern valida el formato "8-4-4-4-12" hexadecimal que arma nextUUID()
// internamente (no es un UUID RFC 4122 real — no fija la versión/variant —
// pero sí el formato visual esperado).
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func buildTempPropertyForEvents(t *testing.T) *domain.Property {
	t.Helper()
	price, err := domain.NewPrice(100, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	loc, err := domain.NewLocation(-34.6, -58.4, "Calle Falsa 123")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	p, err := domain.NewProperty("prop-1", "owner-1", "Depto", "desc", price, loc, domain.OperationTemp, domain.PetPolicyAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	rules := []domain.PricingRule{{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10}}
	tc, err := domain.NewTempConfig(nil, "15:00", "11:00", 2, 30, 50, 20, 100, rules)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	p.SetTempConfig(tc)
	return p
}

// ─── NewPropertyPublished ───────────────────────────────────────────────────

func TestNewPropertyPublished_MapeaSnapshotYMetadataDelEvento(t *testing.T) {
	prop := buildTempPropertyForEvents(t)

	evt := domain.NewPropertyPublished(prop)

	if evt.EventName() != "catalog.property.published" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != prop.ID() {
		t.Errorf("AggregateID: got %q, want %q", evt.AggregateID(), prop.ID())
	}
	if evt.OwnerID != prop.OwnerID() {
		t.Errorf("OwnerID: got %q, want %q", evt.OwnerID, prop.OwnerID())
	}
	if evt.Snapshot.OperationType != string(domain.OperationTemp) || evt.Snapshot.NightPrice != 50 ||
		evt.Snapshot.CleaningFee != 20 || evt.Snapshot.SecurityDeposit != 100 ||
		evt.Snapshot.MinNights != 2 || evt.Snapshot.MaxNights != 30 ||
		evt.Snapshot.CheckInTime != "15:00" || evt.Snapshot.CheckOutTime != "11:00" ||
		len(evt.Snapshot.PricingRules) != 1 {
		t.Errorf("Snapshot: got %+v", evt.Snapshot)
	}
	if !uuidPattern.MatchString(evt.EventID()) {
		t.Errorf("EventID: got %q, no matchea el formato esperado", evt.EventID())
	}
	if evt.OccurredAt().IsZero() || time.Since(evt.OccurredAt()) > time.Minute {
		t.Errorf("OccurredAt: got %v, want un timestamp reciente", evt.OccurredAt())
	}
}

// ─── NewPropertyUpdated ─────────────────────────────────────────────────────

func TestNewPropertyUpdated_MapeaSnapshotYMetadataDelEvento(t *testing.T) {
	prop := buildTempPropertyForEvents(t)

	evt := domain.NewPropertyUpdated(prop)

	if evt.EventName() != "catalog.property.updated" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != prop.ID() {
		t.Errorf("AggregateID: got %q, want %q", evt.AggregateID(), prop.ID())
	}
	if evt.Snapshot.OwnerID != prop.OwnerID() || evt.Snapshot.NightPrice != 50 {
		t.Errorf("Snapshot: got %+v", evt.Snapshot)
	}
}

// ─── NewPropertyDetailsUpdated ──────────────────────────────────────────────

func TestNewPropertyDetailsUpdated_MapeaViejoYNuevoValor(t *testing.T) {
	oldPrice, err := domain.NewPrice(1000, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	newPrice, err := domain.NewPrice(1500, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}

	evt := domain.NewPropertyDetailsUpdated("prop-1", "Título viejo", "Desc vieja", oldPrice, "Título nuevo", "Desc nueva", newPrice)

	if evt.EventName() != "catalog.property.details_updated" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != "prop-1" {
		t.Errorf("AggregateID: got %q", evt.AggregateID())
	}
	if evt.OldTitle != "Título viejo" || evt.NewTitle != "Título nuevo" {
		t.Errorf("Title: got Old=%q New=%q", evt.OldTitle, evt.NewTitle)
	}
	if evt.OldDescription != "Desc vieja" || evt.NewDescription != "Desc nueva" {
		t.Errorf("Description: got Old=%q New=%q", evt.OldDescription, evt.NewDescription)
	}
	if evt.OldPrice != 1000 || evt.NewPrice != 1500 {
		t.Errorf("Price: got Old=%v New=%v", evt.OldPrice, evt.NewPrice)
	}
}

// ─── NewPropertyLocationUpdated ─────────────────────────────────────────────

func TestNewPropertyLocationUpdated_MapeaViejaYNuevaUbicacion(t *testing.T) {
	oldLoc, err := domain.NewLocation(-34.6, -58.4, "Dirección vieja")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	newLoc, err := domain.NewLocation(-31.4, -64.2, "Dirección nueva")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}

	evt := domain.NewPropertyLocationUpdated("prop-1", oldLoc, newLoc)

	if evt.EventName() != "catalog.property.location_updated" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.OldLatitude != -34.6 || evt.OldLongitude != -58.4 || evt.OldAddress != "Dirección vieja" {
		t.Errorf("Old location: got lat=%v lng=%v addr=%q", evt.OldLatitude, evt.OldLongitude, evt.OldAddress)
	}
	if evt.NewLatitude != -31.4 || evt.NewLongitude != -64.2 || evt.NewAddress != "Dirección nueva" {
		t.Errorf("New location: got lat=%v lng=%v addr=%q", evt.NewLatitude, evt.NewLongitude, evt.NewAddress)
	}
}

// ─── NewPropertyPetPolicyUpdated ────────────────────────────────────────────

func TestNewPropertyPetPolicyUpdated_MapeaViejaYNuevaPolitica(t *testing.T) {
	evt := domain.NewPropertyPetPolicyUpdated("prop-1", domain.PetPolicyNotAllowed, domain.PetPolicyAllowed)

	if evt.EventName() != "catalog.property.pet_policy_updated" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.OldPolicy != domain.PetPolicyNotAllowed || evt.NewPolicy != domain.PetPolicyAllowed {
		t.Errorf("Policy: got Old=%q New=%q", evt.OldPolicy, evt.NewPolicy)
	}
}

// ─── NewPropertyTempConfigUpdated ───────────────────────────────────────────

func TestNewPropertyTempConfigUpdated_MapeaLaConfigNueva(t *testing.T) {
	oldConfig, err := domain.NewTempConfig(nil, "14:00", "10:00", 1, 90, 30, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	newConfig, err := domain.NewTempConfig(nil, "16:00", "12:00", 2, 45, 80, 25, 150, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	evt := domain.NewPropertyTempConfigUpdated("prop-1", oldConfig, newConfig)

	if evt.EventName() != "catalog.property.temp_config_updated" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.Snapshot.NightPrice != 80 || evt.Snapshot.CleaningFee != 25 || evt.Snapshot.SecurityDeposit != 150 ||
		evt.Snapshot.MinNights != 2 || evt.Snapshot.MaxNights != 45 ||
		evt.Snapshot.CheckInTime != "16:00" || evt.Snapshot.CheckOutTime != "12:00" {
		t.Errorf("Snapshot: got %+v, want que refleje newConfig", evt.Snapshot)
	}
}

func TestNewPropertyTempConfigUpdated_OldConfigSeIgnoraEnElEvento(t *testing.T) {
	// Pin de comportamiento: el parámetro oldConfig se recibe pero NUNCA se usa
	// en el cuerpo de la función — el evento resultante no lleva ningún rastro
	// del valor anterior (a diferencia de NewPropertyDetailsUpdated o
	// NewPropertyLocationUpdated, que sí exponen Old*/New* explícitamente).
	// Si algún consumidor de este evento necesita diffear valores, hoy no puede.
	oldConfig, err := domain.NewTempConfig(nil, "", "", 1, 90, 999, 999, 999, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	newConfig, err := domain.NewTempConfig(nil, "", "", 1, 90, 1, 1, 1, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	evt := domain.NewPropertyTempConfigUpdated("prop-1", oldConfig, newConfig)

	if evt.Snapshot.NightPrice == 999 {
		t.Fatal("Snapshot no debería reflejar oldConfig bajo ninguna circunstancia")
	}
	if evt.Snapshot.NightPrice != 1 {
		t.Errorf("Snapshot.NightPrice: got %v, want 1 (el valor de newConfig)", evt.Snapshot.NightPrice)
	}
}

// ─── NewPropertyStateChanged ────────────────────────────────────────────────

func TestNewPropertyStateChanged_MapeaViejoYNuevoEstado(t *testing.T) {
	evt := domain.NewPropertyStateChanged("prop-1", domain.StateAvailable, domain.StateReserved)

	if evt.EventName() != "catalog.property.state_changed" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != "prop-1" {
		t.Errorf("AggregateID: got %q", evt.AggregateID())
	}
	if evt.OldState != domain.StateAvailable || evt.NewState != domain.StateReserved {
		t.Errorf("State: got Old=%q New=%q", evt.OldState, evt.NewState)
	}
}

// ─── NewPropertyMediaAdded ──────────────────────────────────────────────────

func TestNewPropertyMediaAdded_MapeaCamposYExtraeS3Key(t *testing.T) {
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "https://mi-bucket.s3.us-east-1.amazonaws.com/properties/prop-1/foto.jpg", domain.MediaTypeImage, 0, nil)
	if err != nil {
		t.Fatalf("NewPropertyMedia: %v", err)
	}

	evt := domain.NewPropertyMediaAdded(media, "owner-1", "mi-bucket", "us-east-1")

	if evt.EventName() != "catalog.property.media_added" {
		t.Errorf("EventName: got %q", evt.EventName())
	}
	if evt.AggregateID() != "prop-1" {
		t.Errorf("AggregateID: got %q, want el PropertyID del media", evt.AggregateID())
	}
	if evt.MediaID != "media-1" || evt.PropertyID != "prop-1" || evt.OwnerID != "owner-1" ||
		evt.BucketName != "mi-bucket" || evt.Region != "us-east-1" || evt.MediaType != string(domain.MediaTypeImage) {
		t.Errorf("evento: got %+v", evt)
	}
	if evt.S3Key != "properties/prop-1/foto.jpg" {
		t.Errorf("S3Key: got %q, want %q", evt.S3Key, "properties/prop-1/foto.jpg")
	}
}

func TestNewPropertyMediaAdded_URLSinMarcadorDeAmazon_S3KeyVacio(t *testing.T) {
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "https://cdn.miapp.com/foto.jpg", domain.MediaTypeImage, 0, nil)
	if err != nil {
		t.Fatalf("NewPropertyMedia: %v", err)
	}

	evt := domain.NewPropertyMediaAdded(media, "owner-1", "mi-bucket", "us-east-1")

	if evt.S3Key != "" {
		t.Errorf("S3Key: got %q, want vacío (la URL no es de S3)", evt.S3Key)
	}
}

func TestNewPropertyMediaAdded_SocialLink_URLVacia_S3KeyVacio(t *testing.T) {
	links := map[string]string{"instagram": "https://instagram.com/depto"}
	media, err := domain.NewPropertyMedia("media-1", "prop-1", "", domain.MediaTypeSocialLink, 0, links)
	if err != nil {
		t.Fatalf("NewPropertyMedia: %v", err)
	}

	evt := domain.NewPropertyMediaAdded(media, "owner-1", "mi-bucket", "us-east-1")

	if evt.S3Key != "" {
		t.Errorf("S3Key: got %q, want vacío (SOCIAL_LINK no tiene URL de archivo)", evt.S3Key)
	}
	if evt.URL != "" {
		t.Errorf("URL: got %q, want vacío", evt.URL)
	}
}

// ─── nextUUID (indirecto) ───────────────────────────────────────────────────

func TestEventos_GeneranEventIDsUnicos(t *testing.T) {
	evt1 := domain.NewPropertyStateChanged("prop-1", domain.StateAvailable, domain.StateReserved)
	evt2 := domain.NewPropertyStateChanged("prop-1", domain.StateAvailable, domain.StateReserved)

	if evt1.EventID() == evt2.EventID() {
		t.Errorf("EventID: dos eventos distintos generaron el mismo ID (%q) — nextUUID no es aleatorio", evt1.EventID())
	}
	if !uuidPattern.MatchString(evt1.EventID()) || !uuidPattern.MatchString(evt2.EventID()) {
		t.Errorf("EventID: got %q / %q, want formato hexadecimal 8-4-4-4-12", evt1.EventID(), evt2.EventID())
	}
}
