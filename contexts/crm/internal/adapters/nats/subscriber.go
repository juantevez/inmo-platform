package nats

import (
	"context"
	"encoding/json"
	"inmo-platform/contexts/crm/internal/application"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type PropertyPublishedPayload struct {
	TargetID string `json:"aggregate_id"`
	OwnerID  string `json:"owner_id"`
}

type PropertyEventSubscriber struct {
	js jetstream.JetStream
	uc *application.CreateAutoLeadUseCase
}

func NewPropertyEventSubscriber(js jetstream.JetStream, uc *application.CreateAutoLeadUseCase) *PropertyEventSubscriber {
	return &PropertyEventSubscriber{js: js, uc: uc}
}

func (s *PropertyEventSubscriber) StartConsume(ctx context.Context) error {
	stream, err := s.js.Stream(ctx, "catalog")
	if err != nil {
		return err
	}

	// 1. Configurar el Consumidor Duradero con Política de Reintentos y DLQ
	consumerCfg := jetstream.ConsumerConfig{
		Durable:       "crm-auto-captacion",
		FilterSubject: "catalog.property.published",
		AckPolicy:     jetstream.AckExplicitPolicy,

		// Reintentar como máximo 3 veces antes de darlo por muerto
		MaxDeliver: 3,

		// Si mandamos un Nack, esperar 5 segundos antes de volver a entregarlo (Backoff simple)
		AckWait: 5 * time.Second,
	}

	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		return err
	}

	log.Println("[NATS CONSUMER] Suscripto con MaxDeliver: 3 y DLQ habilitado nativamente.")

	// 2. Loop de Consumo con manejo explícito de Errores (Ack / Nack)
	_, err = consumer.Consume(func(msg jetstream.Msg) {
		// Obtenemos los metadatos para saber en qué nro de intento vamos
		meta, _ := msg.Metadata()
		deliveryAttempt := meta.NumDelivered

		log.Printf("[NATS CONSUMER] Evento recibido. Intento nro: %d | Payload bytes: %d\n", deliveryAttempt, len(msg.Data()))

		// --- SIMULACIÓN DE ERROR TÉCNICO ---
		// Forzamos un error si el ID contiene la palabra "error" para testear los retries en vivo
		var checkError PropertyPublishedPayload
		_ = json.Unmarshal(msg.Data(), &checkError)

		if checkError.TargetID == "prop-error-poison" {
			log.Printf("[NATS CONSUMER WARN] !!! Fallo simulado para el mensaje tóxico %s. Enviando NACK...\n", checkError.TargetID)
			// Le avisamos a NATS que fallamos. Volverá a intentar según el AckWait
			_ = msg.Nak()
			return
		}
		// ------------------------------------

		// 3. Deserialización normal (Capa Anticorrupción)
		var payload PropertyPublishedPayload
		if err := json.Unmarshal(msg.Data(), &payload); err != nil {
			log.Printf("[NATS ERROR CRÍTICO] JSON malformado. Esto no se puede solucionar reintentando. Enviando Terminate...\n")
			// Term() le dice a NATS: "No me lo mandes más, está roto de origen" (va directo al DLQ si supera MaxDeliver)
			_ = msg.Term()
			return
		}

		// 4. Ejecutar el Caso de Uso de Aplicación
		dto := application.CreateAutoLeadDTO{
			PropertyID: payload.TargetID,
			OwnerID:    payload.OwnerID,
		}

		if err := s.uc.Execute(context.Background(), dto); err != nil {
			log.Printf("[CRM ERROR] Falló el caso de uso transitorio: %v. Reintentando...\n", err)
			_ = msg.Nak() // Reintentar en la próxima pasada
			return
		}

		// 5. ÉXITO TOTAL: Confirmación explícita
		if err := msg.Ack(); err != nil {
			log.Printf("[NATS ERROR] No se pudo enviar el ACK a NATS: %v\n", err)
		}
	})

	return err
}
