package ports

import (
	"context"
	"time"
)

// BlockedDatesRepository gestiona los rangos de fechas no disponibles de una propiedad.
type BlockedDatesRepository interface {
	// HasOverlap reporta si hay algún bloqueo que solape con el rango dado.
	HasOverlap(ctx context.Context, propertyID string, start, end time.Time) (bool, error)
	// Block agrega un rango bloqueado (por reserva o bloqueo manual del dueño).
	Block(ctx context.Context, propertyID, reservationID, reason string, start, end time.Time) error
	// Unblock elimina el bloqueo asociado a una reserva (en caso de cancelación).
	Unblock(ctx context.Context, reservationID string) error
}
