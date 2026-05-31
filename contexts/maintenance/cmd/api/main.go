package main

import (
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq" // Driver oficial de Postgres para database/sql

	"inmo.platform/contexts/maintenance/internal/adapters/httpapi"
	"inmo.platform/contexts/maintenance/internal/adapters/postgres"
	"inmo.platform/contexts/maintenance/internal/application"
)

func main() {
	log.Println("🏁 Iniciando el microservicio de Mantenimiento (Tickets)...")

	// 1. Obtener cadena de conexión desde las variables de entorno o fallback local
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Ajustado a las credenciales reales de tu contenedor
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	// 2. Inicializar base de datos Postgres relacional
	db, err := postgres.InitDB(dbURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar la base de datos de Mantenimiento: %v", err)
	}
	defer db.Close()
	log.Println("🔌 Conexión a PostgreSQL inicializada con éxito para Mantenimiento")

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
