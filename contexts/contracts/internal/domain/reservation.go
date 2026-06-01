package domain

import (
	"time"

	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

type ReservationStatus string

const (
	ReservationPendingApproval ReservationStatus = "PENDING_APPROVAL"
	ReservationConfirmed       ReservationStatus = "CONFIRMED"
	ReservationActive          ReservationStatus = "ACTIVE"
	ReservationCompleted       ReservationStatus = "COMPLETED"
	ReservationCancelled       ReservationStatus = "CANCELLED"
)

// Reservation es el agregado raíz del flujo de reserva temporal.
type Reservation struct {
	ddd.AggregateRoot
	id                  string
	propertyID          string
	tenantID            string
	ownerID             string
	checkInDate         time.Time
	checkOutDate        time.Time
	nights              int
	nightPriceSnapshot  float64
	discountPct         float64
	cleaningFee         float64
	securityDeposit     float64
	totalAmount         float64
	status              ReservationStatus
	guestMessage        string
	confirmedAt         *time.Time
	cancelledAt         *time.Time
	createdAt           time.Time
	updatedAt           time.Time
}

func NewReservation(
	id, propertyID, tenantID, ownerID string,
	checkIn, checkOut time.Time,
	nights int,
	nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount float64,
	guestMessage string,
) (*Reservation, error) {
	if id == "" || propertyID == "" || tenantID == "" || ownerID == "" {
		return nil, apperr.NewBadRequest("id, property_id, tenant_id y owner_id son obligatorios", nil)
	}
	if checkIn.IsZero() || checkOut.IsZero() || !checkOut.After(checkIn) {
		return nil, apperr.NewBadRequest("fechas de check-in y check-out inválidas", nil)
	}
	if nights < 1 {
		return nil, apperr.NewBadRequest("la estadía debe ser de al menos 1 noche", nil)
	}
	now := time.Now().UTC()
	r := &Reservation{
		id:                 id,
		propertyID:         propertyID,
		tenantID:           tenantID,
		ownerID:            ownerID,
		checkInDate:        checkIn,
		checkOutDate:       checkOut,
		nights:             nights,
		nightPriceSnapshot: nightPrice,
		discountPct:        discountPct,
		cleaningFee:        cleaningFee,
		securityDeposit:    securityDeposit,
		totalAmount:        totalAmount,
		status:             ReservationPendingApproval,
		guestMessage:       guestMessage,
		createdAt:          now,
		updatedAt:          now,
	}
	r.RecordEvent(NewReservationCreated(r))
	return r, nil
}

// ReconstructReservation reconstruye el agregado desde persistencia sin disparar eventos.
func ReconstructReservation(
	id, propertyID, tenantID, ownerID string,
	checkIn, checkOut time.Time,
	nights int,
	nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount float64,
	status ReservationStatus,
	guestMessage string,
	confirmedAt, cancelledAt *time.Time,
	createdAt, updatedAt time.Time,
) *Reservation {
	return &Reservation{
		id: id, propertyID: propertyID, tenantID: tenantID, ownerID: ownerID,
		checkInDate: checkIn, checkOutDate: checkOut, nights: nights,
		nightPriceSnapshot: nightPrice, discountPct: discountPct,
		cleaningFee: cleaningFee, securityDeposit: securityDeposit,
		totalAmount: totalAmount, status: status, guestMessage: guestMessage,
		confirmedAt: confirmedAt, cancelledAt: cancelledAt,
		createdAt: createdAt, updatedAt: updatedAt,
	}
}

func (r *Reservation) Confirm() error {
	if r.status != ReservationPendingApproval {
		return apperr.NewPreconditionFailed("solo se puede confirmar una reserva PENDING_APPROVAL", nil)
	}
	now := time.Now().UTC()
	r.status = ReservationConfirmed
	r.confirmedAt = &now
	r.updatedAt = now
	r.RecordEvent(NewReservationConfirmed(r))
	return nil
}

func (r *Reservation) Cancel() error {
	if r.status == ReservationCompleted || r.status == ReservationCancelled {
		return apperr.NewPreconditionFailed("no se puede cancelar una reserva ya finalizada o cancelada", nil)
	}
	now := time.Now().UTC()
	r.status = ReservationCancelled
	r.cancelledAt = &now
	r.updatedAt = now
	r.RecordEvent(NewReservationCancelled(r))
	return nil
}

// Getters
func (r *Reservation) ID() string                      { return r.id }
func (r *Reservation) PropertyID() string              { return r.propertyID }
func (r *Reservation) TenantID() string                { return r.tenantID }
func (r *Reservation) OwnerID() string                 { return r.ownerID }
func (r *Reservation) CheckInDate() time.Time          { return r.checkInDate }
func (r *Reservation) CheckOutDate() time.Time         { return r.checkOutDate }
func (r *Reservation) Nights() int                     { return r.nights }
func (r *Reservation) NightPriceSnapshot() float64    { return r.nightPriceSnapshot }
func (r *Reservation) DiscountPct() float64            { return r.discountPct }
func (r *Reservation) CleaningFee() float64            { return r.cleaningFee }
func (r *Reservation) SecurityDeposit() float64        { return r.securityDeposit }
func (r *Reservation) TotalAmount() float64            { return r.totalAmount }
func (r *Reservation) Status() ReservationStatus       { return r.status }
func (r *Reservation) GuestMessage() string            { return r.guestMessage }
func (r *Reservation) ConfirmedAt() *time.Time         { return r.confirmedAt }
func (r *Reservation) CancelledAt() *time.Time         { return r.cancelledAt }
func (r *Reservation) CreatedAt() time.Time            { return r.createdAt }
func (r *Reservation) UpdatedAt() time.Time            { return r.updatedAt }
