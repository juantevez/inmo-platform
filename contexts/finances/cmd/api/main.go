package main

import (
	"log"
	"net/http"
	"os"

	// Asegurate de importar el driver de postgres para que sql.Open lo reconozca
	_ "github.com/lib/pq"

	"inmo.platform/contexts/finances/internal/adapters/httpapi"
	"inmo.platform/contexts/finances/internal/adapters/postgres"
	"inmo.platform/contexts/finances/internal/application"
)

func main() {
	log.Println("🏁 Iniciando el microservicio de Finanzas...")

	// 1. Obtener cadena de conexión desde las variables de entorno o fallback local
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Ajustá tus credenciales locales de Postgres si corrés nativo en Ubuntu
		dbURL = "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable"
	}

	// 2. Inicializar Infraestructura de Base de Datos
	db, err := postgres.InitDB(dbURL)
	if err != nil {
		log.Fatalf("❌ Error crítico al conectar la base de datos: %v", err)
	}
	defer db.Close()

	// 3. Instanciar Adaptadores de Infraestructura (Persistencia y Stubs de servicios externos)
	settlementRepo := postgres.NewPostgresSettlementRepository(db)
	contractServiceStub := postgres.NewStubContractService()
	eventDispatcherStub := postgres.NewStubEventDispatcher()

	// 4. Instanciar e Inyectar Dependencias en los Casos de Uso (Capa de Aplicación)
	createSettlementUC := application.NewCreateSettlementUseCase(settlementRepo, contractServiceStub)
	addConceptUC := application.NewAddConceptUseCase(settlementRepo)
	closeSettlementUC := application.NewCloseSettlementUseCase(settlementRepo, eventDispatcherStub)

	// 5. Inicializar Capa de Entrada (Controlador HTTP)
	handler := httpapi.NewSettlementHandler(createSettlementUC, addConceptUC, closeSettlementUC)

	// 6. Configurar Enrutador Estándar y Registrar Rutas
	mux := http.NewServeMux()
	httpapi.RegisterSettlementRoutes(mux, handler)

	// 7. Lanzar el Servidor HTTP
	serverPort := ":8082"
	log.Printf("🚀 Servidor de Finanzas escuchando en el puerto %s", serverPort)
	if err := http.ListenAndServe(serverPort, mux); err != nil {
		log.Fatalf("❌ El servidor HTTP falló de forma inesperada: %v", err)
	}

}
