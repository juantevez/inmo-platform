package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	catalogNats "inmo.platform/contexts/catalog/internal/adapters/nats"
	"inmo.platform/contexts/catalog/internal/adapters/postgres"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/shared/pkg/eventbus"
	"inmo.platform/shared/pkg/pg"
)

func main() {
	log.Println("Iniciando Módulo de Catálogo Inmobiliario con Patrón Outbox...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 🚀 CAPTURA DINÁMICA DE ENTORNO
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// 1. Configurar y Conectar Pool de PostgreSQL
	pgConfig := pg.Config{
		URL:          dbURL,
		MaxOpenConns: 25,
		MaxIdleConns: 5,
		MaxIdleTime:  5 * time.Minute,
	}
	dbPool, err := pg.NewPool(pgConfig)
	if err != nil {
		log.Fatalf("No se pudo inicializar el pool de Postgres: %v", err)
	}
	defer dbPool.Close()

	// 2. Configurar y Conectar Broker NATS JetStream
	natsConn, err := eventbus.NewJetStreamConnection(natsURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a NATS JetStream: %v", err)
	}
	defer natsConn.Close()

	// Asegurar stream en NATS
	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	_ = natsConn.EnsureStream(initCtx, "catalog", []string{"catalog.property.*"})
	initCancel()

	// 3. Inicializar el Repositorio de Postgres
	propertyRepo := postgres.NewPropertyRepository(dbPool)

	// 4. Arrancar el Outbox Worker en segundo plano pasándole NATS
	outboxWorker := postgres.NewOutboxWorker(dbPool, natsConn.JS)
	go outboxWorker.Start(ctx, 20*time.Second)

	// =========================================================================
	// 🛠️ PUNTO 4.5: Instanciar y Arrancar el Suscriptor asincrónico de Contratos
	// =========================================================================
	contractSubscriber := catalogNats.NewContractSubscriber(dbPool, natsConn.JS)
	go func() {
		if err := contractSubscriber.StartConsume(ctx); err != nil {
			log.Printf("[CATALOG ERROR] Error crítico en el suscriptor de contratos: %v\n", err)
		}
	}()

	// 5. Inicializar Casos de Uso
	publishUseCase := application.NewPublishPropertyUseCase(dbPool, propertyRepo)
	listUseCase := application.NewListPropertiesUseCase(propertyRepo)

	// 6. Inicializar Adaptadores de Entrada (HTTP API)
	propertyHandler := httpapi.NewPropertyHandler(publishUseCase, nil, listUseCase)
	router := httpapi.NewRouter(propertyHandler)

	// 7. Encender Servidor HTTP (Asignado puerto correcto :8081)
	serverAddr := ":8081"
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Servidor API Catálogo corriendo en el puerto %s\n", serverAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Error crítico en el servidor HTTP: %v", err)
	}
}
