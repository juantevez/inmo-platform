package ports

import (
	"context"
	"database/sql"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
)

// ReservationRepository define el contrato de persistencia del agregado Reservation.
type ReservationRepository interface {
	Save(ctx context.Context, r *domain.Reservation) error
	SaveWithTx(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error
	FindByID(ctx context.Context, id string) (*domain.Reservation, error)
	// HasOverlap verifica si ya existe una reserva CONFIRMED/PENDING para esas fechas.
	HasOverlap(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error)
}

// PropertySnapshotRepository persiste el mirror local de datos de Catálogo.
type PropertySnapshotRepository interface {
	Upsert(ctx context.Context, snap domain.PropertySnapshot) error
	FindByID(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error)
}
