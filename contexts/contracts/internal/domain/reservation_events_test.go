package domain_test

import (
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
)

func newReconstructedReservation(t *testing.T) *domain.Reservation {
	t.Helper()
	checkIn := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	checkOut := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	return domain.ReconstructReservation(
		"res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 5,
		10000, 0, 1500, 5000, 45000,
		domain.ReservationPendingApproval, "",
		nil, nil, time.Now(), time.Now(),
	)
}

func TestNewReservationCreated_MapeaCamposYFormateaLasFechas(t *testing.T) {
	r := newReconstructedReservation(t)

	evt := domain.NewReservationCreated(r)

	if evt.EventName() != "contracts.reservation.created" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.AggregateID() != r.ID() {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), r.ID())
	}
	if evt.PropertyID != "prop-1" || evt.TenantID != "tenant-1" || evt.OwnerID != "owner-1" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", evt)
	}
	if evt.CheckInDate != "2026-08-10" || evt.CheckOutDate != "2026-08-15" {
		t.Fatalf("fechas formateadas incorrectamente: in=%q out=%q", evt.CheckInDate, evt.CheckOutDate)
	}
	if evt.Nights != 5 {
		t.Fatalf("nights: got %d, want 5", evt.Nights)
	}
	if evt.TotalAmount != 45000 {
		t.Fatalf("total amount: got %v, want 45000", evt.TotalAmount)
	}
	if evt.EventID() == "" {
		t.Fatal("EventID no debería estar vacío")
	}
	if evt.OccurredAt().IsZero() {
		t.Fatal("OccurredAt no debería estar vacío")
	}
}

func TestNewReservationConfirmed_MapeaCamposYFormateaLasFechas(t *testing.T) {
	r := newReconstructedReservation(t)

	evt := domain.NewReservationConfirmed(r)

	if evt.EventName() != "contracts.reservation.confirmed" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.AggregateID() != r.ID() {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), r.ID())
	}
	if evt.ReservationID != r.ID() {
		t.Fatalf("reservation id: got %s, want %s", evt.ReservationID, r.ID())
	}
	if evt.PropertyID != "prop-1" || evt.TenantID != "tenant-1" || evt.OwnerID != "owner-1" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", evt)
	}
	if evt.CheckInDate != "2026-08-10" || evt.CheckOutDate != "2026-08-15" {
		t.Fatalf("fechas formateadas incorrectamente: in=%q out=%q", evt.CheckInDate, evt.CheckOutDate)
	}
	if evt.TotalAmount != 45000 {
		t.Fatalf("total amount: got %v, want 45000", evt.TotalAmount)
	}
}

func TestNewReservationCancelled_MapeaCampos(t *testing.T) {
	r := newReconstructedReservation(t)

	evt := domain.NewReservationCancelled(r)

	if evt.EventName() != "contracts.reservation.cancelled" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.AggregateID() != r.ID() {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), r.ID())
	}
	if evt.ReservationID != r.ID() || evt.PropertyID != r.PropertyID() {
		t.Fatalf("identidad mapeada incorrectamente: %+v", evt)
	}
}

func TestReservationEvents_CadaLlamadaGeneraUnEventIDDistinto(t *testing.T) {
	r := newReconstructedReservation(t)

	created := domain.NewReservationCreated(r)
	confirmed := domain.NewReservationConfirmed(r)
	cancelled := domain.NewReservationCancelled(r)

	ids := map[string]bool{created.EventID(): true, confirmed.EventID(): true, cancelled.EventID(): true}
	if len(ids) != 3 {
		t.Fatalf("esperaba 3 EventID distintos, hay %d únicos: %v", len(ids), ids)
	}
}
