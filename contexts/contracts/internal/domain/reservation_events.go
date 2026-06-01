package domain

import (
	"inmo.platform/shared/pkg/ddd"
)

type ReservationCreatedEvent struct {
	ddd.BaseDomainEvent
	PropertyID   string  `json:"property_id"`
	TenantID     string  `json:"tenant_id"`
	OwnerID      string  `json:"owner_id"`
	CheckInDate  string  `json:"check_in_date"`
	CheckOutDate string  `json:"check_out_date"`
	Nights       int     `json:"nights"`
	TotalAmount  float64 `json:"total_amount"`
}

func NewReservationCreated(r *Reservation) ReservationCreatedEvent {
	return ReservationCreatedEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(reservationEventID(), r.ID(), "contracts.reservation.created"),
		PropertyID:      r.PropertyID(),
		TenantID:        r.TenantID(),
		OwnerID:         r.OwnerID(),
		CheckInDate:     r.CheckInDate().Format("2006-01-02"),
		CheckOutDate:    r.CheckOutDate().Format("2006-01-02"),
		Nights:          r.Nights(),
		TotalAmount:     r.TotalAmount(),
	}
}

type ReservationConfirmedEvent struct {
	ddd.BaseDomainEvent
	ReservationID string  `json:"reservation_id"`
	PropertyID    string  `json:"property_id"`
	TenantID      string  `json:"tenant_id"`
	OwnerID       string  `json:"owner_id"`
	CheckInDate   string  `json:"check_in_date"`
	CheckOutDate  string  `json:"check_out_date"`
	TotalAmount   float64 `json:"total_amount"`
}

func NewReservationConfirmed(r *Reservation) ReservationConfirmedEvent {
	return ReservationConfirmedEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(reservationEventID(), r.ID(), "contracts.reservation.confirmed"),
		ReservationID:   r.ID(),
		PropertyID:      r.PropertyID(),
		TenantID:        r.TenantID(),
		OwnerID:         r.OwnerID(),
		CheckInDate:     r.CheckInDate().Format("2006-01-02"),
		CheckOutDate:    r.CheckOutDate().Format("2006-01-02"),
		TotalAmount:     r.TotalAmount(),
	}
}

type ReservationCancelledEvent struct {
	ddd.BaseDomainEvent
	ReservationID string `json:"reservation_id"`
	PropertyID    string `json:"property_id"`
}

func NewReservationCancelled(r *Reservation) ReservationCancelledEvent {
	return ReservationCancelledEvent{
		BaseDomainEvent: ddd.NewBaseDomainEvent(reservationEventID(), r.ID(), "contracts.reservation.cancelled"),
		ReservationID:   r.ID(),
		PropertyID:      r.PropertyID(),
	}
}

// reservationEventID reutiliza nextUUID() definido en events.go del mismo paquete.
func reservationEventID() string { return nextUUID() }
