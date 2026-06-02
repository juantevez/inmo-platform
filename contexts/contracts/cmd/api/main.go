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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	contractsNats "inmo.platform/contexts/contracts/internal/adapters/nats"
	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/shared/pkg/health"
	"inmo.platform/shared/pkg/pg"
)

func main() {
	log.Println("Iniciando Módulo de Gestión de Contratos y Reservas...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	// 1. PostgreSQL
	dbPool, err := pg.NewPool(pg.Config{URL: dbURL, MaxOpenConns: 10, MaxIdleConns: 2, MaxIdleTime: 5 * time.Minute})
	if err != nil {
		log.Fatalf("Contratos: No se pudo conectar a Postgres: %v", err)
	}
	defer dbPool.Close()

	// 2. NATS JetStream
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Contratos: Error al conectar a NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Contratos: Error al inicializar JetStream: %v", err)
	}

	// Streams necesarios
	for _, cfg := range []jetstream.StreamConfig{
		{Name: "contracts", Subjects: []string{"contracts.>"}},
		{Name: "catalog", Subjects: []string{"catalog.property.*"}},
	} {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			log.Fatalf("Contratos: No se pudo crear stream '%s': %v", cfg.Name, err)
		}
	}

	// 3. Repositorios
	contractRepo := postgres.NewContractRepository(dbPool)
	reservationRepo := postgres.NewReservationRepository(dbPool)
	snapshotRepo := postgres.NewSnapshotRepository(dbPool)

	// 4. Goroutines de background con WaitGroup y tracking de estado para health checks
	var wg sync.WaitGroup

	outboxStatus := health.NewWorkerStatus("outbox_worker")
	propertySubStatus := health.NewWorkerStatus("property_subscriber")

	outboxWorker := postgres.NewOutboxWorker(dbPool, js)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer outboxStatus.MarkStopped()
		outboxWorker.Start(ctx, 5*time.Second)
	}()

	propertySub := contractsNats.NewPropertySubscriber(dbPool, js)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer propertySubStatus.MarkStopped()
		if err := propertySub.StartConsume(ctx); err != nil {
			log.Printf("[CONTRACTS ERROR] Subscriber de propiedades: %v\n", err)
		}
	}()

	// 5. Casos de uso — contratos tradicionales
	createUseCase := application.NewCreateContractUseCase(contractRepo)
	activateUseCase := application.NewActivateContractUseCase(dbPool, contractRepo)

	// Casos de uso — reservas temporarias
	createResUC := application.NewCreateReservationUseCase(dbPool, reservationRepo, snapshotRepo)
	confirmResUC := application.NewConfirmReservationUseCase(dbPool, reservationRepo, snapshotRepo)
	cancelResUC := application.NewCancelReservationUseCase(dbPool, reservationRepo)
	getResUC := application.NewGetReservationUseCase(reservationRepo, snapshotRepo)
	listOwnerResUC := application.NewGetOwnerReservationsUseCase(reservationRepo, snapshotRepo)

	// 6. Handlers HTTP
	contractHandler := httpapi.NewContractHandler(createUseCase, activateUseCase)
	reservationHandler := httpapi.NewReservationHandler(createResUC, confirmResUC, cancelResUC, getResUC, listOwnerResUC)
	checker := health.NewChecker(dbPool, nc, outboxStatus, propertySubStatus)
	router := httpapi.NewRouter(contractHandler, reservationHandler, checker)

	server := &http.Server{
		Addr:         ":8085",
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 7. Manejo de señales OS
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Println("Servidor de Contratos y Reservas corriendo en :8085")
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error crítico en el servidor: %v", err)
		}
	case sig := <-quit:
		log.Printf("[CONTRACTS] Señal recibida (%s), iniciando apagado graceful...", sig)
	}

	// 8. Apagado graceful: primero el HTTP, luego las goroutines de background
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[CONTRACTS] Error durante el apagado del servidor HTTP: %v", err)
	}

	cancel()
	wg.Wait()
	log.Println("[CONTRACTS] Módulo apagado correctamente.")
}
