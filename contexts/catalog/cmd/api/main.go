package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	"inmo.platform/contexts/catalog/internal/adapters/inmemory"
	catalogNats "inmo.platform/contexts/catalog/internal/adapters/nats"
	"inmo.platform/contexts/catalog/internal/adapters/postgres"
	s3adapter "inmo.platform/contexts/catalog/internal/adapters/s3"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/eventbus"
	"inmo.platform/shared/pkg/pg"
)

func main() {
	log.Println("Iniciando Módulo de Catálogo Inmobiliario con Patrón Outbox...")
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

	awsBucket := os.Getenv("AWS_BUCKET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}

	// 1. PostgreSQL
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

	// 2. NATS JetStream
	natsConn, err := eventbus.NewJetStreamConnection(natsURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a NATS JetStream: %v", err)
	}
	defer natsConn.Close()

	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	_ = natsConn.EnsureStream(initCtx, "catalog", []string{"catalog.property.*"})
	initCancel()

	// 3. Repositorios
	propertyRepo := postgres.NewPropertyRepository(dbPool)
	profileRepo := postgres.NewPostgresProfileRepository(dbPool)
	mediaRepo := postgres.NewMediaRepository(dbPool)
	blockedDatesRepo := postgres.NewBlockedDatesRepository(dbPool)

	// 4. Outbox Worker + suscriptor de contratos
	outboxWorker := postgres.NewOutboxWorker(dbPool, natsConn.JS)
	go outboxWorker.Start(ctx, 20*time.Second)

	contractSubscriber := catalogNats.NewContractSubscriber(dbPool, natsConn.JS)
	go func() {
		if err := contractSubscriber.StartConsume(ctx); err != nil {
			log.Printf("[CATALOG ERROR] Error crítico en el suscriptor de contratos: %v\n", err)
		}
	}()

	reservationSubscriber := catalogNats.NewReservationSubscriber(dbPool, natsConn.JS)
	go func() {
		if err := reservationSubscriber.StartConsume(ctx); err != nil {
			log.Printf("[CATALOG ERROR] Error crítico en el suscriptor de reservas: %v\n", err)
		}
	}()

	// 5. S3 adapter (opcional: solo se activa si se configuraron las variables de entorno)
	var storageProvider ports.MediaStorageProvider
	if awsBucket != "" {
		s3Adapter, err := s3adapter.NewStorageAdapter(ctx, awsBucket, awsRegion)
		if err != nil {
			log.Printf("[CATALOG WARNING] No se pudo inicializar S3, las subidas de media no estarán disponibles: %v", err)
		} else {
			storageProvider = s3Adapter
			log.Printf("S3 configurado: bucket=%s region=%s", awsBucket, awsRegion)
		}
	} else {
		log.Println("[CATALOG WARNING] AWS_BUCKET_NAME no configurado, endpoint de upload-url no disponible")
	}

	// 6. Casos de uso
	eventPublisher := inmemory.NewEventPublisher()
	publishUseCase := application.NewPublishPropertyUseCase(dbPool, propertyRepo)
	changeStateUseCase := application.NewChangePropertyStateUseCase(propertyRepo, eventPublisher)
	listUseCase := application.NewListPropertiesUseCase(propertyRepo)
	quoteUseCase := application.NewQuotePropertyUseCase(propertyRepo, blockedDatesRepo)
	profileUseCase := application.NewCreateProfileUseCase(profileRepo)
	listMediaUseCase := application.NewListPropertyMediaUseCase(mediaRepo)
	addMediaUseCase := application.NewAddPropertyMediaUseCase(propertyRepo, mediaRepo)
	generateURLUseCase := application.NewGenerateUploadURLUseCase(propertyRepo, storageProvider)

	// 7. Handlers HTTP
	propertyHandler := httpapi.NewPropertyHandler(publishUseCase, changeStateUseCase, listUseCase, quoteUseCase)
	profileHandler := httpapi.NewProfileHandler(profileUseCase)
	mediaHandler := httpapi.NewMediaHandler(generateURLUseCase, addMediaUseCase, listMediaUseCase)

	router := httpapi.NewRouter(propertyHandler, profileHandler, mediaHandler)

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
