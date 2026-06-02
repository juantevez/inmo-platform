package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go/jetstream"
)

// ContractActivatedEvent mapea el espejo de lo que envía el módulo de Contratos
type ContractActivatedEvent struct {
	ID         string `json:"id"`
	PropertyID string `json:"property_id"`
}

type ContractSubscriber struct {
	db *sql.DB
	js jetstream.JetStream // 🚀 Corregido: Usamos el tipo estándar de la interfaz
}

func NewContractSubscriber(db *sql.DB, js jetstream.JetStream) *ContractSubscriber {
	return &ContractSubscriber{
		db: db,
		js: js,
	}
}

// StartConsume inicializa el consumidor durable y bloquea hasta que el contexto se cancele.
func (s *ContractSubscriber) StartConsume(ctx context.Context) error {
	cons, err := s.js.CreateOrUpdateConsumer(ctx, "contracts", jetstream.ConsumerConfig{
		Durable:       "catalog-contract-sync",
		FilterSubject: "contracts.contract.activated",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("error al crear consumidor durable en catálogo: %w", err)
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

	log.Println("[CATALOG NATS] Escuchando firmas de contratos en 'contracts.contract.activated'...")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("error al iterar mensajes: %w", err)
		}

		if err := s.processMessage(ctx, msg); err != nil {
			log.Printf("[CATALOG NATS ERROR] Falló el procesamiento del evento: %v\n", err)
			continue
		}
		_ = msg.Ack()
	}
}

func (s *ContractSubscriber) processMessage(ctx context.Context, msg jetstream.Msg) error {
	var event ContractActivatedEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return fmt.Errorf("error al deserializar evento de contratos: %w", err)
	}

	log.Printf("[CATALOG NATS] ¡Contrato firmado detectado! Dando de baja la propiedad ID: %s\n", event.PropertyID)

	// 3. Ejecutar la mutación en la Base de Datos de Catálogo de forma directa o llamando a un caso de uso
	// 🚀 CORREGIDO: Columna 'state' en lugar de 'status'
	query := `UPDATE properties SET state = 'RENTED', updated_at = CURRENT_TIMESTAMP WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, event.PropertyID)
	if err != nil {
		return fmt.Errorf("error al actualizar estado de la propiedad en bd: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		log.Printf("[CATALOG NATS WARNING] No se encontró ninguna propiedad con ID: %s para dar de baja\n", event.PropertyID)
	} else {
		log.Printf("[CATALOG] Propiedad %s marcada exitosamente como alquilada.\n", event.PropertyID)
	}

	return nil
}
