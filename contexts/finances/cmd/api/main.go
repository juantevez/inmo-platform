package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	// Asegurate de importar el driver de postgres para que sql.Open lo reconozca
	_ "github.com/lib/pq"

	"inmo.platform/contexts/finances/internal/adapters/httpapi"
	"inmo.platform/contexts/finances/internal/adapters/postgres"
	"inmo.platform/contexts/finances/internal/application"
	"inmo.platform/shared/pkg/eventbus"
)

func main() {
	log.Println("🏁 Iniciando el microservicio de Finanzas...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Obtener cadena de conexión desde las variables de entorno o fallback local
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// 2. Inicializar Infraestructura de Base de Datos
	db, err := postgres.InitDB(dbURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar la base de datos: %v", err)
	}
	defer db.Close()

	// 🚀 2.5 Conectar a NATS JetStream para el flujo asincrónico
	natsConn, err := eventbus.NewJetStreamConnection(natsURL)
	if err != nil {
		log.Fatalf("❌ Finanzas no se pudo conectar a NATS JetStream: %v", err)
	}
	defer natsConn.Close()

	// 3. Instanciar Adaptadores de Infraestructura (Persistencia y Stubs)
	settlementRepo := postgres.NewPostgresSettlementRepository(db)
	contractServiceStub := postgres.NewStubContractService()

	// 🚀 NUEVO: Instanciamos el repositorio real del Outbox
	outboxRepo := postgres.NewPostgresOutboxRepository(db)

	// =========================================================================
	// 🛠️ PUNTO 3.5: Instanciar y Arrancar el Outbox Worker para Finanzas
	// =========================================================================
	outboxWorker := postgres.NewOutboxWorker(db, natsConn.JS)
	go outboxWorker.Start(ctx, 10*time.Second)
	// =========================================================================

	// 4. Instanciar e Inyectar Dependencias en los Casos de Uso (Capa de Aplicación)
	createSettlementUC := application.NewCreateSettlementUseCase(settlementRepo, contractServiceStub)
	addConceptUC := application.NewAddConceptUseCase(settlementRepo)

	closeSettlementUC := application.NewCloseSettlementUseCase(settlementRepo, outboxRepo)

	// 5. Inicializar Capa de Entrada (Controlador HTTP)
	handler := httpapi.NewSettlementHandler(createSettlementUC, addConceptUC, closeSettlementUC)

	// 6. Configurar Enrutador Estándar y Registrar Rutas
	mux := http.NewServeMux()
	httpapi.RegisterSettlementRoutes(mux, handler)

	// 7. Lanzar el Servidor HTTP (Mapeado a puerto :8082)
	serverPort := ":8082"
	log.Printf("🚀 Servidor de Finanzas escuchando en el puerto %s", serverPort)
	if err := http.ListenAndServe(serverPort, mux); err != nil {
		log.Fatalf("❌ El servidor HTTP falló de forma inesperada: %v", err)
	}
}
