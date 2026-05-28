package nats

import (
	"context"
	"encoding/json"
	"inmo-platform/contexts/crm/internal/application"
	"log"

	"github.com/nats-io/nats.go/jetstream"
)

// Estructura espejo temporal para deserializar únicamente lo que le importa a CRM del evento original
type PropertyPublishedPayload struct {
	TargetID string `json:"aggregate_id"`
	OwnerID  string `json:"owner_id"`
}

type PropertyEventSubscriber struct {
	js jetstream.JetStream
	uc *application.CreateAutoLeadUseCase
}

func NewPropertyEventSubscriber(js jetstream.JetStream, uc *application.CreateAutoLeadUseCase) *PropertyEventSubscriber {
	return &PropertyEventSubscriber{
		js: js,
		uc: uc,
	}
}

// StartConsume inicializa el loop de escucha asincrónica de manera permanente
func (s *PropertyEventSubscriber) StartConsume(ctx context.Context) error {
	// 1. Obtener una referencia al stream 'catalog' que creamos en el otro módulo
	stream, err := s.js.Stream(ctx, "catalog")
	if err != nil {
		return err
	}

	// 2. Crear o actualizar un Consumidor Duradero (Durable Consumer)
	// Vinculado al subject específico de publicaciones
	consumerCfg := jetstream.ConsumerConfig{
		Durable:       "crm-auto-captacion",
		FilterSubject: "catalog.property.published",
		AckPolicy:     jetstream.AckExplicitPolicy, // Exige que confirmemos la lectura exitosa
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		return err
	}

	log.Println("[NATS CONSUMER] Suscripto exitosamente a 'catalog.property.published' con Durable ID 'crm-auto-captacion'")

	// 3. Iniciar el loop de consumo asincrónico (Consume maneja la concurrencia internamente)
	_, err = consumer.Consume(func(msg jetstream.Msg) {
		// Confirmar siempre la recepción al finalizar el procesamiento (defer)
		defer func() {
			_ = msg.Ack()
		}()

		log.Printf("[NATS CONSUMER] Evento recibido desde JetStream. Payload en bytes: %d\n", len(msg.Data()))

		// 4. Deserializar el evento mapeándolo a nuestro contrato local (Capa Anticorrupción - ACL)
		var payload PropertyPublishedPayload
		if err := json.Unmarshal(msg.Data(), &payload); err != nil {
			log.Printf("[NATS ERROR] Error al parsear el JSON del evento: %v\n", err)
			return
		}

		// 5. Orquestar la acción invocando el caso de uso de Aplicación
		dto := application.CreateAutoLeadDTO{
			PropertyID: payload.TargetID,
			OwnerID:    payload.OwnerID,
		}

		if err := s.uc.Execute(context.Background(), dto); err != nil {
			log.Printf("[CRM ERROR] Falló la ejecución del caso de uso ante evento: %v\n", err)
			return
		}
	})

	return err
}
