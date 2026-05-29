package main

import (
	"context"
	"inmo.platform/contexts/crm/internal/adapters/nats"
	"inmo.platform/contexts/crm/internal/adapters/postgres"
	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/shared/pkg/eventbus"
	"inmo.platform/shared/pkg/pg"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("Iniciando Módulo CRM / Gestión de Leads con Persistencia Real...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Configurar y Conectar Pool de PostgreSQL para CRM
	pgConfig := pg.Config{
		URL:          "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable",
		MaxOpenConns: 10,
		MaxIdleConns: 2,
		MaxIdleTime:  5 * time.Minute,
	}
	dbPool, err := pg.NewPool(pgConfig)
	if err != nil {
		log.Fatalf("CRM no se pudo conectar a Postgres: %v", err)
	}
	defer dbPool.Close()
	log.Println("CRM: Conexión exitosa a PostgreSQL establecida.")

	// 2. Inicializar Conexión al Broker NATS JetStream
	natsURL := "nats://localhost:4222"
	natsConn, err := eventbus.NewJetStreamConnection(natsURL)
	if err != nil {
		log.Fatalf("CRM no se pudo conectar a NATS: %v", err)
	}
	defer natsConn.Close()
	log.Println("CRM conectado exitosamente a NATS Core.")

	// 3. Inicializar Adaptador de Salida Real (Postgres)
	leadRepo := postgres.NewLeadRepository(dbPool)

	// 4. Inicializar Caso de Uso de Aplicación
	createLeadUC := application.NewCreateAutoLeadUseCase(leadRepo)

	// 5. Inicializar Adaptador de Entrada (NATS Subscriber) e iniciar consumo
	subscriber := nats.NewPropertyEventSubscriber(natsConn.JS, createLeadUC)
	if err := subscriber.StartConsume(ctx); err != nil {
		log.Fatalf("Error crítico al iniciar el consumidor de NATS: %v", err)
	}

	log.Println("Módulo CRM activo, persistiendo en BD y escuchando eventos asincrónicos...")

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("Apagando Módulo CRM de manera ordenada...")
}
