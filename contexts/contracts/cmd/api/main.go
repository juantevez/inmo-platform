package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	contractsNats "inmo.platform/contexts/contracts/internal/adapters/nats"
	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/application"
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

	// 4. Outbox Worker
	outboxWorker := postgres.NewOutboxWorker(dbPool, js)
	go outboxWorker.Start(ctx, 5*time.Second)

	// 5. Subscriber: catalog.property.* → actualiza snapshots locales
	propertySub := contractsNats.NewPropertySubscriber(dbPool, js)
	go func() {
		if err := propertySub.StartConsume(ctx); err != nil {
			log.Printf("[CONTRACTS ERROR] Subscriber de propiedades: %v\n", err)
		}
	}()

	// 6. Casos de uso — contratos tradicionales
	createUseCase := application.NewCreateContractUseCase(contractRepo)
	activateUseCase := application.NewActivateContractUseCase(dbPool, contractRepo)

	// Casos de uso — reservas temporarias
	createResUC := application.NewCreateReservationUseCase(dbPool, reservationRepo, snapshotRepo)
	confirmResUC := application.NewConfirmReservationUseCase(dbPool, reservationRepo, snapshotRepo)
	cancelResUC := application.NewCancelReservationUseCase(dbPool, reservationRepo)
	getResUC := application.NewGetReservationUseCase(reservationRepo, snapshotRepo)

	// 7. Handlers HTTP
	contractHandler := httpapi.NewContractHandler(createUseCase, activateUseCase)
	reservationHandler := httpapi.NewReservationHandler(createResUC, confirmResUC, cancelResUC, getResUC)
	router := httpapi.NewRouter(contractHandler, reservationHandler)

	server := &http.Server{
		Addr:         ":8085",
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("Servidor de Contratos y Reservas corriendo en :8085")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Error crítico en el servidor: %v", err)
	}
}
