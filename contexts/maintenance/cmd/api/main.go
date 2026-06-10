package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/maintenance/internal/adapters/httpapi"
	catalogNats "inmo.platform/contexts/maintenance/internal/adapters/nats"
	"inmo.platform/contexts/maintenance/internal/adapters/postgres"
	"inmo.platform/contexts/maintenance/internal/application"
)

func main() {
	log.Println("🏁 Iniciando el microservicio de Mantenimiento (Tickets + Proveedores + Proyecciones)...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// =========================================================================
	// 1. PostgreSQL
	// =========================================================================
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_maintenance_db?sslmode=disable"
	}

	db, err := postgres.InitDB(dbURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar la base de datos de Mantenimiento: %v", err)
	}
	defer db.Close()
	log.Println("🔌 Conexión a PostgreSQL inicializada con éxito para Mantenimiento")

	// =========================================================================
	// 2. NATS JetStream
	// =========================================================================
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(5),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar a NATS Core en Mantenimiento: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("❌ Error crítico al inicializar JetStream en Mantenimiento: %v", err)
	}
	log.Println("🛰️ Conexión a NATS JetStream establecida con éxito para Mantenimiento")

	// Asegurar que el stream "maintenance" existe para los eventos que este módulo publica
	initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
	_, err = js.CreateOrUpdateStream(initCtx, jetstream.StreamConfig{
		Name:      "maintenance",
		Subjects:  []string{"maintenance.ticket.*", "maintenance.provider.*"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	initCancel()
	if err != nil {
		log.Printf("⚠️ No se pudo crear/validar el stream 'maintenance': %v", err)
	}

	// =========================================================================
	// 3. Repositorios
	// =========================================================================
	ticketRepo := postgres.NewPostgresTicketRepository(db)
	providerRepo := postgres.NewPostgresProviderRepository(db)
	projectionRepo := postgres.NewPostgresProjectionRepository(db) // NUEVO

	// =========================================================================
	// 4. Servicios
	// =========================================================================

	// CatalogService usa proyección local para PropertyExists
	// y NATS request/reply para GetPropertyLocation.
	// En desarrollo local sin catalog, usar el Stub.
	var catalogService = catalogNats.NewStubCatalogService(projectionRepo)
	// Para producción con catalog corriendo:
	// var catalogService = catalogNats.NewNatsCatalogService(nc, projectionRepo)

	eventDispatcher := postgres.NewStubEventDispatcher()

	// =========================================================================
	// 5. UUID generator
	// =========================================================================
	uuidGen := func() string {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	}

	// =========================================================================
	// 6. Casos de uso
	// =========================================================================
	createUseCase := application.NewCreateTicketUseCase(ticketRepo, projectionRepo, catalogService)
	assignUseCase := application.NewAssignProviderUseCase(ticketRepo, providerRepo)
	submitUseCase := application.NewSubmitQuoteUseCase(ticketRepo)
	approveUseCase := application.NewApproveTicketUseCase(ticketRepo, eventDispatcher)
	closeUseCase := application.NewCloseTicketUseCase(ticketRepo, eventDispatcher)
	registerProviderUC := application.NewRegisterProviderUseCase(providerRepo, uuidGen)
	listProvidersUC := application.NewListProvidersUseCase(providerRepo)

	// =========================================================================
	// 7. Handlers HTTP
	// =========================================================================
	ticketHandler := httpapi.NewTicketHandler(
		createUseCase,
		assignUseCase,
		submitUseCase,
		approveUseCase,
		closeUseCase,
	)
	providerHandler := httpapi.NewProviderHandler(registerProviderUC, listProvidersUC)

	mux := http.NewServeMux()
	httpapi.MapTicketRoutes(mux, ticketHandler, providerHandler)

	// =========================================================================
	// 8. Goroutines de background con WaitGroup
	// =========================================================================
	var wg sync.WaitGroup

	// Outbox Worker: publica eventos PENDING a NATS
	outboxWorker := postgres.NewOutboxWorker(db, js)
	wg.Add(1)
	go func() {
		defer wg.Done()
		outboxWorker.Start(ctx, 10*time.Second)
		log.Println("⚙️ [MAINTENANCE OUTBOX] Worker detenido.")
	}()
	log.Println("⚙️ [MAINTENANCE OUTBOX] Worker iniciado. Escaneando cada 10s...")

	// CatalogSubscriber: consume catalog.property.published y state_changed
	// para mantener la proyección local sincronizada
	catalogSubscriber := catalogNats.NewCatalogSubscriber(js, projectionRepo)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := catalogSubscriber.StartConsume(ctx); err != nil {
			log.Printf("❌ [MAINTENANCE CATALOG SUB] Error crítico: %v", err)
		}
		log.Println("📡 [MAINTENANCE CATALOG SUB] Subscriber detenido.")
	}()
	log.Println("📡 [MAINTENANCE CATALOG SUB] Subscriber iniciado. Escuchando catalog.property.*...")

	// =========================================================================
	// 9. Servidor HTTP
	// =========================================================================
	serverPort := ":8083"
	server := &http.Server{
		Addr:         serverPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Manejo de señales OS para apagado graceful
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("🚀 Servidor de Mantenimiento escuchando en el puerto %s", serverPort)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Error crítico en el servidor HTTP: %v", err)
		}
	case sig := <-quit:
		log.Printf("[MAINTENANCE] Señal recibida (%s), iniciando apagado graceful...", sig)
	}

	// Apagado graceful: primero HTTP, luego goroutines
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[MAINTENANCE] Error durante el apagado del servidor HTTP: %v", err)
	}

	cancel()  // señal para que las goroutines terminen
	wg.Wait() // esperamos que outbox y subscriber terminen limpio
	log.Println("[MAINTENANCE] Módulo apagado correctamente.")
}
