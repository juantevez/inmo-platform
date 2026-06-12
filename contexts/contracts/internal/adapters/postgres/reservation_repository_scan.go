package postgres

// reservation_repository_scan.go — helper compartido de escaneo de filas.
//
// Extraemos el scanner a un archivo separado para que tanto FindByOwnerID
// (reservation_repository.go) como FindConfirmedCheckingInBetween
// (reservation_repository_reminder.go) lo puedan usar sin duplicar código.
//
// INSTRUCCIÓN DE MIGRACIÓN:
//   En reservation_repository.go, dentro de FindByOwnerID, reemplazá el
//   bloque del bucle rows.Next() + rows.Err() por:
//
//     return scanReservationRows(rows)
//
//   El resultado es idéntico funcionalmente; solo eliminás la duplicación.

import (
	"database/sql"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// scanReservationRows convierte un *sql.Rows en un slice de *domain.Reservation.
// Asume que las columnas vienen en el orden estándar definido en los queries
// de FindByOwnerID y FindConfirmedCheckingInBetween.
func scanReservationRows(rows *sql.Rows) ([]*domain.Reservation, error) {
	var result []*domain.Reservation

	for rows.Next() {
		var (
			resID, propID, tenantID, ownerID, status   string
			checkIn, checkOut                           time.Time
			nights                                      int
			nightPrice, discPct, cleaning, deposit, tot float64
			guestMsg                                    sql.NullString
			confirmedAt, cancelledAt                    sql.NullTime
			createdAt, updatedAt                        time.Time
		)
		if err := rows.Scan(
			&resID, &propID, &tenantID, &ownerID,
			&checkIn, &checkOut, &nights,
			&nightPrice, &discPct, &cleaning, &deposit, &tot,
			&status, &guestMsg, &confirmedAt, &cancelledAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, apperr.NewInternal("error al escanear fila de reserva", err)
		}

		var confAt, canAt *time.Time
		if confirmedAt.Valid {
			t := confirmedAt.Time
			confAt = &t
		}
		if cancelledAt.Valid {
			t := cancelledAt.Time
			canAt = &t
		}

		result = append(result, domain.ReconstructReservation(
			resID, propID, tenantID, ownerID, checkIn, checkOut, nights,
			nightPrice, discPct, cleaning, deposit, tot,
			domain.ReservationStatus(status), guestMsg.String,
			confAt, canAt, createdAt, updatedAt,
		))
	}

	if err := rows.Err(); err != nil {
		return nil, apperr.NewInternal("error al iterar filas de reservas", err)
	}

	return result, nil
}
