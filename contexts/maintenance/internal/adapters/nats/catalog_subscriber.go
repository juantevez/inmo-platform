package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/maintenance/internal/domain"
	"inmo.platform/contexts/maintenance/internal/ports"
)

// propertyPublishedEvent mapea el payload de catalog.property.published.
// Solo extraemos los campos que necesitamos para la proyección local —
// el resto (snapshot con precios, amenities, etc.) lo ignoramos.
type propertyPublishedEvent struct {
	EventID     string           `json:"event_id"`
	AggregateID string           `json:"aggregate_id"` // property_id
	EventName   string           `json:"event_name"`
	OwnerID     string           `json:"owner_id"`
	Snapshot    propertySnapshot `json:"snapshot"`
}

type propertySnapshot struct {
	OwnerID       string `json:"owner_id"`
	OperationType string `json:"operation_type"`
}

// propertyStateChangedEvent mapea el payload de catalog.property.state_changed.
type propertyStateChangedEvent struct {
	EventID     string `json:"event_id"`
	AggregateID string `json:"aggregate_id"` // property_id
	EventName   string `json:"event_name"`
	NewState    string `json:"new_state"`
}

// CatalogSubscriber consume eventos de catalog para mantener la proyección local
// de propiedades en inmo_maintenance_db.
//
// Subjects que consume:
//   - catalog.property.published    → Upsert en property_projections
//   - catalog.property.state_changed → UpdateState en property_projections
//
// Consumer durable: "maintenance-catalog-sync"
// Stream esperado: "catalog" (creado por el módulo de catálogo)
type CatalogSubscriber struct {
	js             jetstream.JetStream
	projectionRepo ports.PropertyProjectionRepository
}

func NewCatalogSubscriber(js jetstream.JetStream, projectionRepo ports.PropertyProjectionRepository) *CatalogSubscriber {
	return &CatalogSubscriber{
		js:             js,
		projectionRepo: projectionRepo,
	}
}

// StartConsume inicia el consumidor durable y bloquea hasta que el contexto se cancele.
// Debe correrse en una goroutine separada desde main.go.
func (s *CatalogSubscriber) StartConsume(ctx context.Context) error {
	// Consumer durable con filtro en ambos subjects de catalog que nos interesan
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "catalog", jetstream.ConsumerConfig{
		Durable: "maintenance-catalog-sync",
		FilterSubjects: []string{
			"catalog.property.published",
			"catalog.property.state_changed",
		},
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("[MAINTENANCE CATALOG SUB] error al crear consumidor durable: %w", err)
	}

	iter, err := cons.Messages()
	if err != nil {
		return fmt.Errorf("[MAINTENANCE CATALOG SUB] error al obtener iterador: %w", err)
	}
	defer iter.Stop()

	// Detener el iterador cuando el contexto se cancele
	go func() {
		<-ctx.Done()
		iter.Stop()
	}()

	log.Println("[MAINTENANCE CATALOG SUB] Escuchando catalog.property.published y catalog.property.state_changed...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				log.Println("[MAINTENANCE CATALOG SUB] Contexto cancelado, deteniendo subscriber.")
				return nil
			}
			return fmt.Errorf("[MAINTENANCE CATALOG SUB] error al iterar mensajes: %w", err)
		}

		subject := msg.Subject()
		var processErr error

		switch subject {
		case "catalog.property.published":
			processErr = s.handlePropertyPublished(ctx, msg.Data())
		case "catalog.property.state_changed":
			processErr = s.handleStateChanged(ctx, msg.Data())
		default:
			log.Printf("[MAINTENANCE CATALOG SUB] Subject desconocido ignorado: %s", subject)
		}

		if processErr != nil {
			// No hacemos Ack — NATS reintentará el mensaje en el próximo ciclo
			log.Printf("[MAINTENANCE CATALOG SUB ERROR] subject=%s err=%v", subject, processErr)
			continue
		}

		_ = msg.Ack()
	}
}

// handlePropertyPublished procesa catalog.property.published y hace Upsert en la proyección.
func (s *CatalogSubscriber) handlePropertyPublished(ctx context.Context, data []byte) error {
	var event propertyPublishedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("error al deserializar catalog.property.published: %w", err)
	}

	// Usamos el owner_id del snapshot si viene ahí, sino del campo raíz del evento
	ownerID := event.Snapshot.OwnerID
	if ownerID == "" {
		ownerID = event.OwnerID
	}

	projection := domain.NewPropertyProjection(
		event.AggregateID, // property_id
		ownerID,
		event.Snapshot.OperationType,
		"AVAILABLE", // toda propiedad publicada nace como AVAILABLE
		"",          // tenant_id: no está en el snapshot actual (Opción B)
	)

	if err := s.projectionRepo.Upsert(ctx, projection); err != nil {
		return fmt.Errorf("error al hacer Upsert de proyección para propiedad %s: %w", event.AggregateID, err)
	}

	log.Printf("[MAINTENANCE CATALOG SUB] Proyección creada/actualizada: property_id=%s owner_id=%s op=%s",
		event.AggregateID, ownerID, event.Snapshot.OperationType)
	return nil
}

// handleStateChanged procesa catalog.property.state_changed y actualiza solo el estado.
func (s *CatalogSubscriber) handleStateChanged(ctx context.Context, data []byte) error {
	var event propertyStateChangedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("error al deserializar catalog.property.state_changed: %w", err)
	}

	if err := s.projectionRepo.UpdateState(ctx, event.AggregateID, event.NewState); err != nil {
		return fmt.Errorf("error al actualizar estado de proyección %s → %s: %w",
			event.AggregateID, event.NewState, err)
	}

	log.Printf("[MAINTENANCE CATALOG SUB] Estado actualizado: property_id=%s new_state=%s",
		event.AggregateID, event.NewState)
	return nil
}
