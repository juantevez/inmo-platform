package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/application"

	"inmo.platform/shared/pkg/pg"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	log.Println("Iniciando Módulo de Gestión de Contratos Inmobiliarios...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 🚀 CAPTURA DINÁMICA DE ENTORNO
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL // nats://127.0.0.1:4222
	}

	// 1. PostgreSQL
	pgConfig := pg.Config{
		URL:          dbURL,
		MaxOpenConns: 10,
		MaxIdleConns: 2,
		MaxIdleTime:  5 * time.Minute,
	}
	dbPool, err := pg.NewPool(pgConfig)
	if err != nil {
		log.Fatalf("Contratos: No se pudo conectar a Postgres: %v", err)
	}
	defer dbPool.Close()

	// 2. NATS Core & JetStream
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Contratos: Error al conectar a NATS Core: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Contratos: Error al inicializar JetStream: %v", err)
	}

	// Declarar el Stream para Contratos
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "contracts",
		Subjects: []string{"contracts.>"},
	})
	if err != nil {
		log.Fatalf("Contratos: No se pudo crear el Stream en NATS: %v", err)
	}

	// 3. Repositorio e Inyección de Dependencias
	contractRepo := postgres.NewContractRepository(dbPool)
	createUseCase := application.NewCreateContractUseCase(contractRepo)
	activateUseCase := application.NewActivateContractUseCase(dbPool, contractRepo)

	// 4. Encender el Outbox Worker en background
	outboxWorker := postgres.NewOutboxWorker(dbPool, js)
	go outboxWorker.Start(ctx, 5*time.Second)

	// 5. Servidor HTTP (Mapeado al puerto correcto :8085)
	contractHandler := httpapi.NewContractHandler(createUseCase, activateUseCase)
	router := httpapi.NewRouter(contractHandler)

	server := &http.Server{
		Addr:         ":8085",
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("Servidor de Contratos corriendo exitosamente en el puerto :8085")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Error crítico en el servidor de Contratos: %v", err)
	}
}
