package domain

import "time"

// PropertyProjection es la copia local mínima de una propiedad en el módulo de mantenimiento.
//
// Solo contiene los datos que maintenance necesita para operar de forma autónoma:
//   - owner_id: para notificar al propietario cuando hay un presupuesto pendiente de aprobar
//   - operation_type: para determinar si la propiedad puede tener tickets activos (ej: TEMP tiene restricciones)
//   - state: para validar que no se abran tickets en propiedades CLOSED
//
// Datos que NO se almacenan acá (se consultan via NATS request/reply cuando se necesitan):
//   - address / latitud / longitud: el técnico los pide al llegar a la orden de trabajo
//   - title / description: solo relevantes para el catálogo
//   - price: no tiene relación con mantenimiento
type PropertyProjection struct {
	propertyID    string
	ownerID       string
	operationType string
	state         string
	tenantID      string
	syncedAt      time.Time
	createdAt     time.Time
}

// NewPropertyProjection construye la proyección a partir del evento recibido de catalog.
func NewPropertyProjection(propertyID, ownerID, operationType, state, tenantID string) *PropertyProjection {
	now := time.Now()
	return &PropertyProjection{
		propertyID:    propertyID,
		ownerID:       ownerID,
		operationType: operationType,
		state:         state,
		tenantID:      tenantID,
		syncedAt:      now,
		createdAt:     now,
	}
}

// ReconstructPropertyProjection rehidrata la proyección desde Postgres.
func ReconstructPropertyProjection(
	propertyID, ownerID, operationType, state, tenantID string,
	syncedAt, createdAt time.Time,
) *PropertyProjection {
	return &PropertyProjection{
		propertyID:    propertyID,
		ownerID:       ownerID,
		operationType: operationType,
		state:         state,
		tenantID:      tenantID,
		syncedAt:      syncedAt,
		createdAt:     createdAt,
	}
}

// UpdateState actualiza el estado cuando llega un evento catalog.property.state_changed.
func (p *PropertyProjection) UpdateState(newState string) {
	p.state = newState
	p.syncedAt = time.Now()
}

// CanHaveActiveTicket valida si la propiedad acepta nuevos tickets de mantenimiento.
// Una propiedad CLOSED o sin proyección no debería recibir tickets nuevos.
func (p *PropertyProjection) CanHaveActiveTicket() bool {
	return p.state != "CLOSED"
}

// Getters
func (p *PropertyProjection) PropertyID() string    { return p.propertyID }
func (p *PropertyProjection) OwnerID() string       { return p.ownerID }
func (p *PropertyProjection) OperationType() string { return p.operationType }
func (p *PropertyProjection) State() string         { return p.state }
func (p *PropertyProjection) TenantID() string      { return p.tenantID }
func (p *PropertyProjection) SyncedAt() time.Time   { return p.syncedAt }
func (p *PropertyProjection) CreatedAt() time.Time  { return p.createdAt }
