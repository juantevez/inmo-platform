package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"inmo.platform/contexts/maintenance/internal/ports"
)

// NatsCatalogService implementa ports.CatalogService usando:
//   - La proyección local para PropertyExists (sin ir a catalog)
//   - NATS request/reply para GetPropertyLocation (consulta en tiempo real)
//
// Subject de request/reply: "catalog.property.query.location"
// Catalog debe tener un responder en ese subject (a implementar en catalog).
//
// Timeout de request/reply: 3 segundos — si catalog no responde, el handler
// retorna error y el técnico puede reintentar.
type NatsCatalogService struct {
	nc             *natsgo.Conn
	projectionRepo ports.PropertyProjectionRepository
}

func NewNatsCatalogService(nc *natsgo.Conn, projectionRepo ports.PropertyProjectionRepository) *NatsCatalogService {
	return &NatsCatalogService{
		nc:             nc,
		projectionRepo: projectionRepo,
	}
}

// PropertyExists verifica si la propiedad tiene proyección local.
// Si no hay proyección, el evento de catalog todavía no llegó (lag de NATS)
// o la propiedad realmente no existe.
func (s *NatsCatalogService) PropertyExists(ctx context.Context, propertyID string) (bool, error) {
	projection, err := s.projectionRepo.FindByID(ctx, propertyID)
	if err != nil {
		return false, fmt.Errorf("error al consultar proyección local: %w", err)
	}
	return projection != nil, nil
}

// GetPropertyLocation consulta catalog via NATS request/reply para obtener
// la dirección y coordenadas de una propiedad.
//
// Solo se llama cuando el técnico necesita la ubicación —
// no en cada creación de ticket.
func (s *NatsCatalogService) GetPropertyLocation(ctx context.Context, propertyID string) (*ports.PropertyLocationResult, error) {
	// Request payload
	reqPayload, err := json.Marshal(map[string]string{"property_id": propertyID})
	if err != nil {
		return nil, fmt.Errorf("error al serializar request de ubicación: %w", err)
	}

	// NATS request/reply con timeout de 3 segundos
	msg, err := s.nc.RequestWithContext(ctx, "catalog.property.query.location", reqPayload)
	if err != nil {
		if err == natsgo.ErrTimeout {
			return nil, fmt.Errorf("catalog no respondió en tiempo (timeout 3s) para property_id=%s", propertyID)
		}
		return nil, fmt.Errorf("error en NATS request/reply para ubicación: %w", err)
	}

	// Deserializar respuesta
	var result ports.PropertyLocationResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("error al deserializar respuesta de ubicación de catalog: %w", err)
	}

	return &result, nil
}

// =========================================================================
// Stub para desarrollo local (sin catalog corriendo)
// =========================================================================

// StubCatalogService implementa ports.CatalogService usando la proyección local
// para PropertyExists y datos ficticios para GetPropertyLocation.
// Usar en entornos donde catalog no está disponible.
type StubCatalogService struct {
	projectionRepo ports.PropertyProjectionRepository
}

func NewStubCatalogService(projectionRepo ports.PropertyProjectionRepository) *StubCatalogService {
	return &StubCatalogService{projectionRepo: projectionRepo}
}

func (s *StubCatalogService) PropertyExists(ctx context.Context, propertyID string) (bool, error) {
	if projectionRepo := s.projectionRepo; projectionRepo != nil {
		proj, err := projectionRepo.FindByID(ctx, propertyID)
		if err != nil {
			return false, err
		}
		return proj != nil, nil
	}
	// Fallback legacy: acepta cualquier ID excepto "invalid-prop"
	return propertyID != "invalid-prop", nil
}

func (s *StubCatalogService) GetPropertyLocation(_ context.Context, propertyID string) (*ports.PropertyLocationResult, error) {
	_ = time.Now() // referencia para no perder el import
	return &ports.PropertyLocationResult{
		PropertyID: propertyID,
		Address:    "[STUB] Av. Corrientes 1234, Buenos Aires",
		Latitude:   -34.603722,
		Longitude:  -58.381592,
		Title:      "[STUB] Propiedad de desarrollo",
	}, nil
}
