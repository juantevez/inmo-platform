package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"inmo.platform/contexts/crm/internal/adapters/httpapi"
	"inmo.platform/contexts/crm/internal/adapters/nats"
	"inmo.platform/contexts/crm/internal/adapters/postgres"
	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/shared/pkg/eventbus"
	"inmo.platform/shared/pkg/health"
	"inmo.platform/shared/pkg/pg"
)

func main() {
	log.Println("Iniciando Módulo CRM / Gestión de Leads con Persistencia Real...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// 1. Configurar y Conectar Pool de PostgreSQL para CRM
	pgConfig := pg.Config{
		URL:          dbURL,
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
	natsConn, err := eventbus.NewJetStreamConnection(natsURL)
	if err != nil {
		log.Fatalf("CRM no se pudo conectar a NATS: %v", err)
	}
	defer natsConn.Close()
	log.Println("CRM conectado exitosamente a NATS Core.")

	// 2.1. Asegurar los streams necesarios. CRM publica al stream "crm" (antes
	// dependía implícitamente de que contexts/chat lo creara primero) y
	// re-asegura defensivamente "catalog", del que consume.
	if err := natsConn.EnsureStream(ctx, "crm", []string{"crm.>"}); err != nil {
		log.Fatalf("CRM no pudo asegurar el stream 'crm': %v", err)
	}
	if err := natsConn.EnsureStream(ctx, "catalog", []string{"catalog.property.*"}); err != nil {
		log.Fatalf("CRM no pudo asegurar el stream 'catalog': %v", err)
	}

	// 3. Inicializar Adaptador de Salida Real (Postgres)
	leadRepo := postgres.NewPostgresLeadRepository(dbPool)

	// 4. Goroutines de background
	var wg sync.WaitGroup

	outboxStatus := health.NewWorkerStatus("outbox_worker")
	propertySubStatus := health.NewWorkerStatus("property_subscriber")

	// 4.1. Outbox publisher
	outboxWorker := postgres.NewOutboxWorker(dbPool, natsConn.JS)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer outboxStatus.MarkStopped()
		outboxWorker.Start(ctx, 15*time.Second)
	}()

	// 4.2. Captación automática de leads reactiva a catalog.property.published
	createAutoLeadUC := application.NewCreateAutoLeadUseCase(leadRepo)
	subscriber := nats.NewPropertyEventSubscriber(natsConn.JS, createAutoLeadUC)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer propertySubStatus.MarkStopped()
		if err := subscriber.StartConsume(ctx); err != nil {
			log.Printf("[CRM ERROR] Subscriber de propiedades: %v\n", err)
		}
	}()

	// 5. Casos de uso — seguimiento manual del agente
	getLeadUC := application.NewGetLeadUseCase(leadRepo)
	contactUC := application.NewContactLeadUseCase(leadRepo)
	scheduleUC := application.NewScheduleVisitUseCase(leadRepo)
	closeUC := application.NewCloseLeadUseCase(leadRepo)

	// 6. Handler HTTP y router
	leadHandler := httpapi.NewLeadHandler(getLeadUC, contactUC, scheduleUC, closeUC)
	checker := health.NewChecker(dbPool, natsConn.NC, outboxStatus, propertySubStatus)
	router := httpapi.NewRouter(leadHandler, checker)

	server := &http.Server{
		Addr:         ":8084",
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 7. Manejo de señales OS
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Println("Servidor de CRM corriendo en :8084")
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error crítico en el servidor: %v", err)
		}
	case sig := <-quit:
		log.Printf("[CRM] Señal recibida (%s), iniciando apagado graceful...", sig)
	}

	// 8. Apagado graceful
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[CRM] Error durante el apagado HTTP: %v", err)
	}

	cancel()
	wg.Wait()
	log.Println("[CRM] Módulo apagado correctamente.")
}
