package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// InitDB inicializa el pool de conexiones a PostgreSQL
func InitDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("error al abrir la base de datos: %w", err)
	}

	// Configuración del pool para entornos estables
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verificar la conexión real
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("no se pudo hacer ping a la base de datos: %w", err)
	}

	log.Println("🔌 Conexión a PostgreSQL inicializada con éxito para Finanzas")
	return db, nil
}

// =========================================================================
// STUBS DE INTEGRACIÓN (Para probar el flujo de Aplicación sin depender de NATS/Microservicios)
// =========================================================================

// StubContractService emula la comunicación vía gRPC/HTTP con el módulo de Contratos
type StubContractService struct{}

func NewStubContractService() *StubContractService {
	return &StubContractService{}
}

func (s *StubContractService) IsContractActive(ctx context.Context, contractID string) (bool, error) {
	log.Printf("[🛰️ Stub Contract] Verificando estado del contrato: %s\n", contractID)

	// Regla de simulación: Si el ID empieza con "invalid", simulamos que no existe o está caído.
	// Cualquier otro UUID/ID va a pasar de largo como activo.
	if contractID == "invalid-contract-id" {
		return false, nil
	}

	return true, nil
}

// StubEventDispatcher emula la publicación de eventos de dominio en NATS Streaming / Kafka
type StubEventDispatcher struct{}

func NewStubEventDispatcher() *StubEventDispatcher {
	return &StubEventDispatcher{}
}

func (s *StubEventDispatcher) PublishSettlementClosed(ctx context.Context, settlementID string) error {
	log.Println("=======================================================================")
	log.Printf("📢 [🔥 STUB EVENT] Evitiendo evento: 'SettlementClosedEvent'\n")
	log.Printf("🆔 Liquidación ID: %s\n", settlementID)
	log.Printf("💡 Acción: Notificando a Propietario e Inquilino para descarga de liquidación.\n")
	log.Println("=======================================================================")
	return nil
}
