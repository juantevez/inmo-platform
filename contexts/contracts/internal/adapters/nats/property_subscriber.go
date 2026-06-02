package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/domain"
)

// propertySnapshotPayload refleja el campo "snapshot" del evento catalog.property.published/updated.
type propertySnapshotPayload struct {
	OwnerID         string        `json:"owner_id"`
	OperationType   string        `json:"operation_type"`
	NightPrice      float64       `json:"night_price"`
	CleaningFee     float64       `json:"cleaning_fee"`
	SecurityDeposit float64       `json:"security_deposit"`
	MinNights       int           `json:"min_nights"`
	MaxNights       int           `json:"max_nights"`
	CheckInTime     string        `json:"check_in_time"`
	CheckOutTime    string        `json:"check_out_time"`
	PricingRules    []pricingRule `json:"pricing_rules"`
}

type pricingRule struct {
	Type        string  `json:"type"`
	MinNights   int     `json:"min_nights"`
	DiscountPct float64 `json:"discount_pct"`
}

type catalogPropertyEvent struct {
	// BaseDomainEvent serializa TargetID como "aggregate_id" (ver shared/pkg/ddd/event.go)
	AggregateID string                  `json:"aggregate_id"`
	Snapshot    propertySnapshotPayload `json:"snapshot"`
}

// PropertySubscriber escucha eventos de Catálogo y mantiene los snapshots actualizados en Contratos.
type PropertySubscriber struct {
	snapRepo *postgres.SnapshotRepository
	js       jetstream.JetStream
}

func NewPropertySubscriber(db *sql.DB, js jetstream.JetStream) *PropertySubscriber {
	return &PropertySubscriber{
		snapRepo: postgres.NewSnapshotRepository(db),
		js:       js,
	}
}

func (s *PropertySubscriber) StartConsume(ctx context.Context) error {
	// Escucha tanto property.published como property.updated con un filtro wildcard
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "catalog", jetstream.ConsumerConfig{
		Durable:       "contracts-property-sync",
		FilterSubject: "catalog.property.*",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("error al crear consumidor de propiedades en contratos: %w", err)
	}

	iter, err := cons.Messages()
	if err != nil {
		return err
	}
	defer iter.Stop()

	// Desbloquea iter.Next() cuando el contexto se cancele.
	go func() {
		<-ctx.Done()
		iter.Stop()
	}()

	log.Println("[CONTRACTS NATS] Escuchando snapshots de propiedades en 'catalog.property.*'...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("error al iterar eventos de catálogo: %w", err)
		}
		if err := s.processMessage(ctx, msg); err != nil {
			log.Printf("[CONTRACTS NATS ERROR] Error al procesar snapshot: %v\n", err)
			continue
		}
		_ = msg.Ack()
	}
}

func (s *PropertySubscriber) processMessage(ctx context.Context, msg jetstream.Msg) error {
	var event catalogPropertyEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return fmt.Errorf("error al deserializar evento de catálogo: %w", err)
	}

	// Solo sincronizamos propiedades de tipo TEMP
	if event.Snapshot.OperationType != "TEMP" {
		return nil
	}

	rules := make([]domain.PricingRule, 0, len(event.Snapshot.PricingRules))
	for _, r := range event.Snapshot.PricingRules {
		rules = append(rules, domain.PricingRule{
			Type:        r.Type,
			MinNights:   r.MinNights,
			DiscountPct: r.DiscountPct,
		})
	}

	snap := domain.PropertySnapshot{
		PropertyID:      event.AggregateID,
		OwnerID:         event.Snapshot.OwnerID,
		OperationType:   event.Snapshot.OperationType,
		NightPrice:      event.Snapshot.NightPrice,
		CleaningFee:     event.Snapshot.CleaningFee,
		SecurityDeposit: event.Snapshot.SecurityDeposit,
		MinNights:       event.Snapshot.MinNights,
		MaxNights:       event.Snapshot.MaxNights,
		CheckInTime:     event.Snapshot.CheckInTime,
		CheckOutTime:    event.Snapshot.CheckOutTime,
		PricingRules:    rules,
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.snapRepo.Upsert(ctx, snap); err != nil {
		return err
	}

	log.Printf("[CONTRACTS] Snapshot actualizado para propiedad %s (TEMP, $%.2f/noche)\n",
		event.AggregateID, snap.NightPrice)
	return nil
}
