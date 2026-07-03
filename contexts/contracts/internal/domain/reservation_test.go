package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func assertPreconditionFailed(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func newReservationParams() (checkIn, checkOut time.Time, nights int, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount float64, guestMessage string) {
	checkIn = time.Now().Add(24 * time.Hour)
	checkOut = checkIn.Add(3 * 24 * time.Hour)
	return checkIn, checkOut, 3, 10000, 10, 1500, 5000, 32000, "Llegamos de noche, ¿hay problema?"
}

func newValidReservation(t *testing.T) *domain.Reservation {
	t.Helper()
	checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()
	r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage)
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}
	return r
}

// ─── NewReservation ─────────────────────────────────────────────────────────

func TestNewReservation_MapeaCamposYNaceEnPendingApproval(t *testing.T) {
	checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()

	r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage)
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}

	if r.Status() != domain.ReservationPendingApproval {
		t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationPendingApproval)
	}
	if r.ID() != "res-1" || r.PropertyID() != "prop-1" || r.TenantID() != "tenant-1" || r.OwnerID() != "owner-1" {
		t.Fatalf("getters de identidad mapeados incorrectamente: %+v", r)
	}
	if !r.CheckInDate().Equal(checkIn) || !r.CheckOutDate().Equal(checkOut) {
		t.Fatalf("fechas mapeadas incorrectamente")
	}
	if r.Nights() != nights || r.NightPriceSnapshot() != nightPrice || r.DiscountPct() != discountPct ||
		r.CleaningFee() != cleaningFee || r.SecurityDeposit() != securityDeposit || r.TotalAmount() != totalAmount {
		t.Fatalf("montos mapeados incorrectamente: %+v", r)
	}
	if r.GuestMessage() != guestMessage {
		t.Fatalf("guest message: got %q, want %q", r.GuestMessage(), guestMessage)
	}
	if r.ConfirmedAt() != nil || r.CancelledAt() != nil {
		t.Fatal("una reserva nueva no debería tener confirmedAt/cancelledAt")
	}
	if !r.CreatedAt().Equal(r.UpdatedAt()) {
		t.Fatalf("createdAt y updatedAt deberían coincidir al crear: %v vs %v", r.CreatedAt(), r.UpdatedAt())
	}
}

func TestNewReservation_RegistraEventoReservationCreated(t *testing.T) {
	r := newValidReservation(t)

	events := r.PullEvents()
	if len(events) != 1 {
		t.Fatalf("esperaba 1 evento, hay %d", len(events))
	}
	evt, ok := events[0].(domain.ReservationCreatedEvent)
	if !ok {
		t.Fatalf("tipo de evento incorrecto: %T", events[0])
	}
	if evt.EventName() != "contracts.reservation.created" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.AggregateID() != r.ID() {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), r.ID())
	}
	if evt.PropertyID != r.PropertyID() || evt.TenantID != r.TenantID() || evt.OwnerID != r.OwnerID() {
		t.Fatalf("identidad mapeada incorrectamente en el evento: %+v", evt)
	}
	if evt.CheckInDate != r.CheckInDate().Format("2006-01-02") || evt.CheckOutDate != r.CheckOutDate().Format("2006-01-02") {
		t.Fatalf("fechas formateadas incorrectamente en el evento: %+v", evt)
	}
	if evt.Nights != r.Nights() || evt.TotalAmount != r.TotalAmount() {
		t.Fatalf("nights/total mapeados incorrectamente en el evento: %+v", evt)
	}
}

func TestNewReservation_CamposObligatoriosFaltantes_RetornaBadRequest(t *testing.T) {
	checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()

	tests := []struct {
		name                              string
		id, propertyID, tenantID, ownerID string
	}{
		{"sin id", "", "prop-1", "tenant-1", "owner-1"},
		{"sin property_id", "res-1", "", "tenant-1", "owner-1"},
		{"sin tenant_id", "res-1", "prop-1", "", "owner-1"},
		{"sin owner_id", "res-1", "prop-1", "tenant-1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := domain.NewReservation(tc.id, tc.propertyID, tc.tenantID, tc.ownerID,
				checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage)
			assertBadRequest(t, err)
			if r != nil {
				t.Fatal("esperaba nil ante un error de validación")
			}
		})
	}
}

func TestNewReservation_FechasInvalidas_RetornaBadRequest(t *testing.T) {
	checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()

	tests := []struct {
		name              string
		checkIn, checkOut time.Time
	}{
		{"check-in vacío", time.Time{}, checkOut},
		{"check-out vacío", checkIn, time.Time{}},
		{"check-out igual a check-in", checkIn, checkIn},
		{"check-out anterior a check-in", checkOut, checkIn},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
				tc.checkIn, tc.checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage)
			assertBadRequest(t, err)
			if r != nil {
				t.Fatal("esperaba nil ante un error de validación")
			}
		})
	}
}

func TestNewReservation_NochesMenorAUno_RetornaBadRequest(t *testing.T) {
	checkIn, checkOut, _, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()

	for _, nights := range []int{0, -1} {
		r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
			checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage)
		assertBadRequest(t, err)
		if r != nil {
			t.Fatalf("noches=%d: esperaba nil ante un error de validación", nights)
		}
	}
}

// ─── ReconstructReservation ─────────────────────────────────────────────────

func TestReconstructReservation_MapeaTodoSinDispararEventos(t *testing.T) {
	checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()
	confirmedAt := time.Now().Add(-2 * time.Hour)
	createdAt := time.Now().Add(-48 * time.Hour)
	updatedAt := time.Now().Add(-1 * time.Hour)

	r := domain.ReconstructReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount,
		domain.ReservationConfirmed, guestMessage, &confirmedAt, nil, createdAt, updatedAt)

	if r.Status() != domain.ReservationConfirmed {
		t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationConfirmed)
	}
	if r.ConfirmedAt() == nil || !r.ConfirmedAt().Equal(confirmedAt) {
		t.Fatalf("confirmedAt mapeado incorrectamente: %v", r.ConfirmedAt())
	}
	if r.CancelledAt() != nil {
		t.Fatal("cancelledAt debería ser nil")
	}
	if !r.CreatedAt().Equal(createdAt) || !r.UpdatedAt().Equal(updatedAt) {
		t.Fatalf("createdAt/updatedAt mapeados incorrectamente")
	}
	if len(r.PullEvents()) != 0 {
		t.Fatal("ReconstructReservation no debería registrar eventos de dominio")
	}
}

// ─── Confirm ────────────────────────────────────────────────────────────────

func TestConfirm_DesdePendingApproval_ConfirmaYRegistraEvento(t *testing.T) {
	r := newValidReservation(t)
	r.PullEvents() // limpiar el evento de creación

	before := time.Now().UTC()
	if err := r.Confirm(); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if r.Status() != domain.ReservationConfirmed {
		t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationConfirmed)
	}
	if r.ConfirmedAt() == nil || r.ConfirmedAt().Before(before) {
		t.Fatalf("confirmedAt no se seteó correctamente: %v", r.ConfirmedAt())
	}
	if !r.UpdatedAt().Equal(*r.ConfirmedAt()) {
		t.Fatalf("updatedAt debería coincidir con confirmedAt: %v vs %v", r.UpdatedAt(), r.ConfirmedAt())
	}

	events := r.PullEvents()
	if len(events) != 1 {
		t.Fatalf("esperaba 1 evento, hay %d", len(events))
	}
	evt, ok := events[0].(domain.ReservationConfirmedEvent)
	if !ok {
		t.Fatalf("tipo de evento incorrecto: %T", events[0])
	}
	if evt.EventName() != "contracts.reservation.confirmed" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.ReservationID != r.ID() || evt.PropertyID != r.PropertyID() || evt.TenantID != r.TenantID() || evt.OwnerID != r.OwnerID() {
		t.Fatalf("identidad mapeada incorrectamente en el evento: %+v", evt)
	}
	if evt.TotalAmount != r.TotalAmount() {
		t.Fatalf("total amount mapeado incorrectamente: got %v, want %v", evt.TotalAmount, r.TotalAmount())
	}
}

func TestConfirm_DesdeEstadoNoPending_RetornaPreconditionFailed(t *testing.T) {
	tests := []domain.ReservationStatus{
		domain.ReservationConfirmed, domain.ReservationActive,
		domain.ReservationCompleted, domain.ReservationCancelled,
	}

	for _, status := range tests {
		t.Run(string(status), func(t *testing.T) {
			checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()
			r := domain.ReconstructReservation("res-1", "prop-1", "tenant-1", "owner-1",
				checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount,
				status, guestMessage, nil, nil, time.Now(), time.Now())

			err := r.Confirm()
			assertPreconditionFailed(t, err)
			if r.Status() != status {
				t.Fatalf("el estado no debería cambiar tras el error: got %s", r.Status())
			}
			if len(r.PullEvents()) != 0 {
				t.Fatal("no debería registrarse ningún evento si Confirm falla")
			}
		})
	}
}

// ─── Cancel ─────────────────────────────────────────────────────────────────

func TestCancel_DesdeEstadosCancelables_CancelaYRegistraEvento(t *testing.T) {
	tests := []domain.ReservationStatus{
		domain.ReservationPendingApproval, domain.ReservationConfirmed, domain.ReservationActive,
	}

	for _, status := range tests {
		t.Run(string(status), func(t *testing.T) {
			checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()
			r := domain.ReconstructReservation("res-1", "prop-1", "tenant-1", "owner-1",
				checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount,
				status, guestMessage, nil, nil, time.Now(), time.Now())

			before := time.Now().UTC()
			if err := r.Cancel(); err != nil {
				t.Fatalf("Cancel: %v", err)
			}

			if r.Status() != domain.ReservationCancelled {
				t.Fatalf("status: got %s, want %s", r.Status(), domain.ReservationCancelled)
			}
			if r.CancelledAt() == nil || r.CancelledAt().Before(before) {
				t.Fatalf("cancelledAt no se seteó correctamente: %v", r.CancelledAt())
			}

			events := r.PullEvents()
			if len(events) != 1 {
				t.Fatalf("esperaba 1 evento, hay %d", len(events))
			}
			evt, ok := events[0].(domain.ReservationCancelledEvent)
			if !ok {
				t.Fatalf("tipo de evento incorrecto: %T", events[0])
			}
			if evt.EventName() != "contracts.reservation.cancelled" {
				t.Fatalf("event name: got %s", evt.EventName())
			}
			if evt.ReservationID != r.ID() || evt.PropertyID != r.PropertyID() {
				t.Fatalf("identidad mapeada incorrectamente en el evento: %+v", evt)
			}
		})
	}
}

func TestCancel_DesdeEstadoFinalizado_RetornaPreconditionFailed(t *testing.T) {
	tests := []domain.ReservationStatus{domain.ReservationCompleted, domain.ReservationCancelled}

	for _, status := range tests {
		t.Run(string(status), func(t *testing.T) {
			checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount, guestMessage := newReservationParams()
			r := domain.ReconstructReservation("res-1", "prop-1", "tenant-1", "owner-1",
				checkIn, checkOut, nights, nightPrice, discountPct, cleaningFee, securityDeposit, totalAmount,
				status, guestMessage, nil, nil, time.Now(), time.Now())

			err := r.Cancel()
			assertPreconditionFailed(t, err)
			if r.Status() != status {
				t.Fatalf("el estado no debería cambiar tras el error: got %s", r.Status())
			}
			if len(r.PullEvents()) != 0 {
				t.Fatal("no debería registrarse ningún evento si Cancel falla")
			}
		})
	}
}
