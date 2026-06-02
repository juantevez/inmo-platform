package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type reservationConfirmedEvent struct {
	ReservationID string `json:"reservation_id"`
	PropertyID    string `json:"property_id"`
	CheckInDate   string `json:"check_in_date"`  // YYYY-MM-DD
	CheckOutDate  string `json:"check_out_date"` // YYYY-MM-DD
}

// ReservationSubscriber escucha eventos de reserva confirmada desde Contratos
// y bloquea las fechas correspondientes en property_blocked_dates.
type ReservationSubscriber struct {
	db *sql.DB
	js jetstream.JetStream
}

func NewReservationSubscriber(db *sql.DB, js jetstream.JetStream) *ReservationSubscriber {
	return &ReservationSubscriber{db: db, js: js}
}

func (s *ReservationSubscriber) StartConsume(ctx context.Context) error {
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "contracts", jetstream.ConsumerConfig{
		Durable:       "catalog-reservation-sync",
		FilterSubject: "contracts.reservation.confirmed",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("error al crear consumidor de reservas en catálogo: %w", err)
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

	log.Println("[CATALOG NATS] Escuchando reservas confirmadas en 'contracts.reservation.confirmed'...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("error al iterar mensajes de reservas: %w", err)
		}
		if err := s.processMessage(ctx, msg); err != nil {
			log.Printf("[CATALOG NATS ERROR] Falló el bloqueo de fechas: %v\n", err)
			continue
		}
		_ = msg.Ack()
	}
}

func (s *ReservationSubscriber) processMessage(ctx context.Context, msg jetstream.Msg) error {
	var event reservationConfirmedEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return fmt.Errorf("error al deserializar reservation.confirmed: %w", err)
	}

	start, err := time.Parse("2006-01-02", event.CheckInDate)
	if err != nil {
		return fmt.Errorf("check_in_date inválida: %w", err)
	}
	end, err := time.Parse("2006-01-02", event.CheckOutDate)
	if err != nil {
		return fmt.Errorf("check_out_date inválida: %w", err)
	}

	id := fmt.Sprintf("blk-%s-%d", event.PropertyID, time.Now().UnixNano())
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO property_blocked_dates (id, property_id, start_date, end_date, reason, reservation_id)
		VALUES ($1, $2, $3, $4, 'RESERVATION', $5)
		ON CONFLICT DO NOTHING`,
		id, event.PropertyID, start.Format("2006-01-02"), end.Format("2006-01-02"), event.ReservationID,
	)
	if err != nil {
		return fmt.Errorf("error al insertar fechas bloqueadas para propiedad %s: %w", event.PropertyID, err)
	}

	log.Printf("[CATALOG] Fechas bloqueadas: propiedad=%s reserva=%s del %s al %s\n",
		event.PropertyID, event.ReservationID, event.CheckInDate, event.CheckOutDate)
	return nil
}
