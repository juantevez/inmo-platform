package postgres

import (
	"context"
	"log"

	"inmo.platform/contexts/maintenance/internal/domain"
)

// StubCatalogService simula la comunicación síncrona (gRPC/HTTP) con Catálogo
type StubCatalogService struct{}

func NewStubCatalogService() *StubCatalogService {
	return &StubCatalogService{}
}

func (s *StubCatalogService) PropertyExists(ctx context.Context, propertyID string) (bool, error) {
	log.Printf("[🛰️ Stub Catalog] Verificando existencia de la propiedad: %s", propertyID)
	// Simulamos que cualquier ID que no sea "invalid-prop" es válido
	if propertyID == "invalid-prop" {
		return false, nil
	}
	return true, nil
}

// StubEventDispatcher simula la publicación de eventos de integración al Broker (NATS/Kafka)
type StubEventDispatcher struct{}

func NewStubEventDispatcher() *StubEventDispatcher {
	return &StubEventDispatcher{}
}

func (s *StubEventDispatcher) DispatchApproved(ctx context.Context, event domain.TicketApprovedEvent) error {
	log.Println("=======================================================================")
	log.Printf("📢 [🔥 MAINTENANCE EVENT] Emitiendo: 'TicketApprovedEvent'")
	log.Printf("🆔 Ticket ID: %s | 🏠 Propiedad ID: %s", event.TicketID, event.PropertyID)
	log.Printf("💡 Acción Catálogo: Moviendo estado de propiedad a 'EN REPARACIÓN'")
	log.Println("=======================================================================")
	return nil
}

func (s *StubEventDispatcher) DispatchClosed(ctx context.Context, event domain.TicketClosedEvent) error {
	log.Println("=======================================================================")
	log.Printf("📢 [🔥 MAINTENANCE EVENT] Emitiendo: 'TicketClosedEvent'")
	log.Printf("🆔 Ticket ID: %s | 🏠 Propiedad ID: %s | 💵 Costo Final: $%.2f", event.TicketID, event.PropertyID, event.Cost)
	log.Printf("💡 Acción Catálogo: Devolviendo propiedad a 'DISPONIBLE' e indexando historial")
	log.Println("=======================================================================")
	return nil
}
