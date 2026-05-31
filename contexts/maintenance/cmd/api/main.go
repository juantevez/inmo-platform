package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq" // Driver oficial de Postgres para database/sql
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/maintenance/internal/adapters/httpapi"
	"inmo.platform/contexts/maintenance/internal/adapters/postgres"
	"inmo.platform/contexts/maintenance/internal/application"
)

func main() {
	log.Println("🏁 Iniciando el microservicio de Mantenimiento (Tickets)...")

	// 0. Crear el contexto global para el ciclo de vida de la aplicación y el Worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Obtener cadena de conexión desde las variables de entorno o fallback local
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Corregido para que apunte a su base de datos correspondiente por defecto
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_maintenance_db?sslmode=disable"
	}

	// 2. Inicializar base de datos Postgres relacional
	db, err := postgres.InitDB(dbURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar la base de datos de Mantenimiento: %v", err)
	}
	defer db.Close()
	log.Println("🔌 Conexión a PostgreSQL inicializada con éxito para Mantenimiento")

	// 2.5. Inicializar la conexión a NATS para el Outbox Worker
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL // "nats://localhost:4222"
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar a NATS Core en Mantenimiento: %v", err)
	}
	defer nc.Close()

	// Instanciamos la API moderna de JetStream (idéntico a como gestionás los otros servicios)
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("❌ Error crítico al inicializar JetStream en Mantenimiento: %v", err)
	}
	log.Println("🛰️ Conexión a NATS JetStream establecida con éxito para Mantenimiento")

	// =========================================================================
	// 🛠️ Instanciar y Arrancar el Outbox Worker para Mantenimiento
	// =========================================================================
	// Ahora 'db', 'js' y 'ctx' están perfectamente definidos y al alcance
	outboxWorker := postgres.NewOutboxWorker(db, js)
	go outboxWorker.Start(ctx, 10*time.Second)
	log.Println("⚙️ [MAINTENANCE OUTBOX] Worker iniciado con éxito. Escaneando cada 10s...")
	// =========================================================================

	// 3. Inicializar Adaptadores (Infraestructura y Stubs)
	ticketRepo := postgres.NewPostgresTicketRepository(db)
	catalogService := postgres.NewStubCatalogService()
	eventDispatcher := postgres.NewStubEventDispatcher()

	// 4. Inicializar Casos de Uso (Inyección de Dependencias)
	createUseCase := application.NewCreateTicketUseCase(ticketRepo, catalogService)
	assignUseCase := application.NewAssignProviderUseCase(ticketRepo)
	submitUseCase := application.NewSubmitQuoteUseCase(ticketRepo)
	approveUseCase := application.NewApproveTicketUseCase(ticketRepo, eventDispatcher)
	closeUseCase := application.NewCloseTicketUseCase(ticketRepo, eventDispatcher)

	// 5. Configurar Adaptador de Entrada (HTTP API)
	handler := httpapi.NewTicketHandler(
		createUseCase,
		assignUseCase,
		submitUseCase,
		approveUseCase,
		closeUseCase,
	)

	mux := http.NewServeMux()
	httpapi.MapTicketRoutes(mux, handler)

	// 6. Arrancar Servidor HTTP en el puerto asignado (8083 para evitar colisiones)
	serverPort := ":8083"
	server := &http.Server{
		Addr:         serverPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("🚀 Servidor de Mantenimiento escuchando en el puerto %s", serverPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Error al arrancar el servidor HTTP: %v", err)
	}
}
