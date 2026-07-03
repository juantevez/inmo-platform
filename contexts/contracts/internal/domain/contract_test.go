package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
)

func mustRent(t *testing.T, amount float64, currency string) domain.RentAmount {
	t.Helper()
	r, err := domain.NewRentAmount(amount, currency)
	if err != nil {
		t.Fatalf("NewRentAmount: %v", err)
	}
	return r
}

func mustTimeline(t *testing.T) domain.Timeline {
	t.Helper()
	tl, err := domain.NewTimeline(time.Now(), time.Now().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("NewTimeline: %v", err)
	}
	return tl
}

func newDraftContract(t *testing.T) *domain.Contract {
	t.Helper()
	return domain.NewContract(
		"contract-1", "prop-1", "tenant-1", "owner-1",
		mustRent(t, 100000, "ARS"), mustTimeline(t),
		domain.IndexICL, domain.AdjustmentPeriod(4),
	)
}

// ─── NewContract ────────────────────────────────────────────────────────────

func TestNewContract_NaceEnEstadoDraft(t *testing.T) {
	c := newDraftContract(t)

	if c.State() != domain.StateDraft {
		t.Fatalf("state: got %s, want %s", c.State(), domain.StateDraft)
	}
	if c.ID() != "contract-1" || c.PropertyID() != "prop-1" || c.TenantID() != "tenant-1" || c.OwnerID() != "owner-1" {
		t.Fatalf("getters mapeados incorrectamente: %+v", c)
	}
	if len(c.PullEvents()) != 0 {
		t.Fatal("NewContract no debería registrar eventos de dominio")
	}
}

func TestNewContract_MapeaRentTimelineIndexYPeriod(t *testing.T) {
	rent := mustRent(t, 50000, "USD")
	tl := mustTimeline(t)
	c := domain.NewContract("c-1", "p-1", "t-1", "o-1", rent, tl, domain.IndexIPC, domain.AdjustmentPeriod(6))

	if c.RentAmount() != rent {
		t.Fatalf("RentAmount: got %+v, want %+v", c.RentAmount(), rent)
	}
	if c.Timeline() != tl {
		t.Fatalf("Timeline: got %+v, want %+v", c.Timeline(), tl)
	}
	if c.Index() != domain.IndexIPC {
		t.Fatalf("Index: got %s, want %s", c.Index(), domain.IndexIPC)
	}
	if c.AdjustmentPeriod() != domain.AdjustmentPeriod(6) {
		t.Fatalf("AdjustmentPeriod: got %d, want %d", c.AdjustmentPeriod(), 6)
	}
}

// ─── Activate ───────────────────────────────────────────────────────────────

func TestActivate_DesdeDraft_ActivaYRegistraEvento(t *testing.T) {
	c := newDraftContract(t)

	if err := c.Activate(); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if c.State() != domain.StateActive {
		t.Fatalf("state: got %s, want %s", c.State(), domain.StateActive)
	}

	events := c.PullEvents()
	if len(events) != 1 {
		t.Fatalf("esperaba 1 evento, hay %d", len(events))
	}
	evt, ok := events[0].(domain.ContractActivated)
	if !ok {
		t.Fatalf("tipo de evento incorrecto: %T", events[0])
	}
	if evt.EventName() != "contracts.contract.activated" {
		t.Fatalf("event name: got %s", evt.EventName())
	}
	if evt.AggregateID() != c.ID() {
		t.Fatalf("aggregate id: got %s, want %s", evt.AggregateID(), c.ID())
	}
	if evt.PropertyID != c.PropertyID() {
		t.Fatalf("property id: got %s, want %s", evt.PropertyID, c.PropertyID())
	}
}

func TestActivate_DesdeEstadoNoDraft_RetornaErrInvalidStateTransition(t *testing.T) {
	tests := []struct {
		name  string
		state domain.ContractState
	}{
		{"desde Active", domain.StateActive},
		{"desde Renewed", domain.StateRenewed},
		{"desde Terminated", domain.StateTerminated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := domain.RehydrateContract(
				"c-1", "p-1", "t-1", "o-1",
				mustRent(t, 1000, "ARS"), mustTimeline(t),
				domain.IndexFix, domain.AdjustmentPeriod(0),
				tc.state, time.Now(),
			)

			err := c.Activate()
			if !errors.Is(err, domain.ErrInvalidStateTransition) {
				t.Fatalf("got %v, want ErrInvalidStateTransition", err)
			}
			if c.State() != tc.state {
				t.Fatalf("el estado no debería haber cambiado tras el error: got %s", c.State())
			}
			if len(c.PullEvents()) != 0 {
				t.Fatal("no debería registrarse ningún evento si Activate falla")
			}
		})
	}
}

// ─── ApplyAdjustment ────────────────────────────────────────────────────────

func TestApplyAdjustment_ContratoActivo_AplicaElCoeficiente(t *testing.T) {
	c := newDraftContract(t)
	if err := c.Activate(); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	c.PullEvents() // limpiar el evento de Activate, no es lo que estamos probando acá

	if err := c.ApplyAdjustment(1.5); err != nil {
		t.Fatalf("ApplyAdjustment: %v", err)
	}

	if got, want := c.RentAmount().Amount(), 150000.0; got != want {
		t.Fatalf("rent amount: got %v, want %v", got, want)
	}
}

func TestApplyAdjustment_ContratoNoActivo_RetornaError(t *testing.T) {
	tests := []domain.ContractState{domain.StateDraft, domain.StateRenewed, domain.StateTerminated}

	for _, state := range tests {
		t.Run(string(state), func(t *testing.T) {
			c := domain.RehydrateContract(
				"c-1", "p-1", "t-1", "o-1",
				mustRent(t, 1000, "ARS"), mustTimeline(t),
				domain.IndexFix, domain.AdjustmentPeriod(0),
				state, time.Now(),
			)

			if err := c.ApplyAdjustment(1.5); err == nil {
				t.Fatal("esperaba error, contrato no está ACTIVO")
			}
			if got, want := c.RentAmount().Amount(), 1000.0; got != want {
				t.Fatalf("el monto no debería cambiar: got %v, want %v", got, want)
			}
		})
	}
}

func TestApplyAdjustment_CoeficienteInvalido_RetornaError(t *testing.T) {
	tests := []float64{0, -1, -0.5}

	for _, coef := range tests {
		c := newDraftContract(t)
		if err := c.Activate(); err != nil {
			t.Fatalf("Activate: %v", err)
		}

		if err := c.ApplyAdjustment(coef); err == nil {
			t.Fatalf("coeficiente %v: esperaba error", coef)
		}
		if got, want := c.RentAmount().Amount(), 100000.0; got != want {
			t.Fatalf("coeficiente %v: el monto no debería cambiar, got %v, want %v", coef, got, want)
		}
	}
}

// ─── RehydrateContract ──────────────────────────────────────────────────────

func TestRehydrateContract_ReconstruyeSinDispararEventos(t *testing.T) {
	createdAt := time.Now().Add(-48 * time.Hour)
	c := domain.RehydrateContract(
		"c-1", "p-1", "t-1", "o-1",
		mustRent(t, 2000, "ARS"), mustTimeline(t),
		domain.IndexICL, domain.AdjustmentPeriod(4),
		domain.StateActive, createdAt,
	)

	if c.State() != domain.StateActive {
		t.Fatalf("state: got %s, want %s", c.State(), domain.StateActive)
	}
	if !c.CreatedAt().Equal(createdAt) {
		t.Fatalf("createdAt: got %v, want %v", c.CreatedAt(), createdAt)
	}
	if len(c.PullEvents()) != 0 {
		t.Fatal("RehydrateContract no debería registrar eventos de dominio")
	}
}
