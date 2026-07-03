package domain_test

import (
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func mustNewLead(t *testing.T) *domain.Lead {
	t.Helper()
	l, err := domain.NewLead("lead-1", "prop-1", "Juan", "juan@test.com", "")
	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	return l
}

// ─── NewLead ────────────────────────────────────────────────────────────────

func TestNewLead_ConEmailYTelefono_CreaElLeadEnEstadoNew(t *testing.T) {
	before := time.Now()
	l, err := domain.NewLead("lead-1", "prop-1", "Juan Pérez", "juan@test.com", "+54911234567")
	after := time.Now()

	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	if l.ID != "lead-1" || l.PropertyID != "prop-1" || l.ClientName != "Juan Pérez" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", l)
	}
	if l.Email != "juan@test.com" || l.Phone != "+54911234567" {
		t.Fatalf("contacto mapeado incorrectamente: %+v", l)
	}
	if l.State != domain.StateNew {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateNew)
	}
	if l.VisitScheduledAt != nil {
		t.Fatalf("un lead nuevo no debería tener visita agendada: %v", l.VisitScheduledAt)
	}
	if l.CreatedAt.Before(before) || l.CreatedAt.After(after) {
		t.Fatalf("CreatedAt fuera del rango esperado: %v", l.CreatedAt)
	}
	// NewLead llama time.Now() por separado para cada campo, así que pueden
	// diferir en nanosegundos — solo importa que ambos caigan en la ventana
	// de creación, no que sean bit-a-bit idénticos.
	if l.UpdatedAt.Before(before) || l.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt fuera del rango esperado: %v", l.UpdatedAt)
	}
}

func TestNewLead_SoloConEmail_Ok(t *testing.T) {
	l, err := domain.NewLead("lead-1", "prop-1", "Juan", "juan@test.com", "")
	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	if l.Email != "juan@test.com" || l.Phone != "" {
		t.Fatalf("contacto mapeado incorrectamente: %+v", l)
	}
}

func TestNewLead_SoloConTelefono_Ok(t *testing.T) {
	l, err := domain.NewLead("lead-1", "prop-1", "Juan", "", "+54911234567")
	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	if l.Phone != "+54911234567" || l.Email != "" {
		t.Fatalf("contacto mapeado incorrectamente: %+v", l)
	}
}

func TestNewLead_SinID_RetornaBadRequest(t *testing.T) {
	l, err := domain.NewLead("", "prop-1", "Juan", "juan@test.com", "")
	assertBadRequest(t, err)
	if l != nil {
		t.Fatalf("esperaba nil ante un error de validación, obtuve %+v", l)
	}
}

func TestNewLead_SinPropertyID_RetornaBadRequest(t *testing.T) {
	l, err := domain.NewLead("lead-1", "", "Juan", "juan@test.com", "")
	assertBadRequest(t, err)
	if l != nil {
		t.Fatalf("esperaba nil ante un error de validación, obtuve %+v", l)
	}
}

func TestNewLead_SinEmailNiTelefono_RetornaBadRequest(t *testing.T) {
	l, err := domain.NewLead("lead-1", "prop-1", "Juan", "", "")
	assertBadRequest(t, err)
	if l != nil {
		t.Fatalf("esperaba nil ante un error de validación, obtuve %+v", l)
	}
}

// ─── MarkContacted ──────────────────────────────────────────────────────────

func TestMarkContacted_DesdeNew_CambiaDeEstadoYActualizaUpdatedAt(t *testing.T) {
	l := mustNewLead(t)
	originalUpdatedAt := l.UpdatedAt
	time.Sleep(time.Millisecond)

	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted: %v", err)
	}
	if l.State != domain.StateContacted {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateContacted)
	}
	if !l.UpdatedAt.After(originalUpdatedAt) {
		t.Fatalf("UpdatedAt debería haber avanzado: got %v, want > %v", l.UpdatedAt, originalUpdatedAt)
	}
}

func TestMarkContacted_DesdeEstadoNoNew_RetornaErrInvalidTransition(t *testing.T) {
	tests := []domain.LeadState{domain.StateContacted, domain.StateVisitScheduled, domain.StateClosed}

	for _, state := range tests {
		t.Run(string(state), func(t *testing.T) {
			l := mustNewLead(t)
			l.State = state // fixture directa: partimos de un estado ya avanzado

			err := l.MarkContacted()
			if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Fatalf("got %v, want ErrInvalidTransition", err)
			}
			if l.State != state {
				t.Fatalf("el estado no debería cambiar tras el error: got %s", l.State)
			}
		})
	}
}

// ─── ScheduleVisit ──────────────────────────────────────────────────────────

func TestScheduleVisit_DesdeContacted_FechaFutura_Ok(t *testing.T) {
	l := mustNewLead(t)
	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted setup: %v", err)
	}
	visitAt := time.Now().Add(48 * time.Hour)

	if err := l.ScheduleVisit(visitAt); err != nil {
		t.Fatalf("ScheduleVisit: %v", err)
	}
	if l.State != domain.StateVisitScheduled {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateVisitScheduled)
	}
	if l.VisitScheduledAt == nil || !l.VisitScheduledAt.Equal(visitAt) {
		t.Fatalf("VisitScheduledAt mapeado incorrectamente: %v", l.VisitScheduledAt)
	}
}

func TestScheduleVisit_FechaPasada_RetornaBadRequest(t *testing.T) {
	l := mustNewLead(t)
	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted setup: %v", err)
	}

	err := l.ScheduleVisit(time.Now().Add(-1 * time.Hour))
	assertBadRequest(t, err)
	if l.State != domain.StateContacted {
		t.Fatalf("el estado no debería cambiar tras el error: got %s", l.State)
	}
	if l.VisitScheduledAt != nil {
		t.Fatal("VisitScheduledAt no debería setearse tras el error")
	}
}

func TestScheduleVisit_FechaExactamenteAhora_RetornaBadRequest(t *testing.T) {
	// La invariante exige estrictamente "después de ahora": el instante actual
	// no cuenta como futuro.
	l := mustNewLead(t)
	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted setup: %v", err)
	}

	err := l.ScheduleVisit(time.Now())
	assertBadRequest(t, err)
}

func TestScheduleVisit_DesdeEstadoNoContacted_RetornaErrInvalidTransition(t *testing.T) {
	tests := []domain.LeadState{domain.StateNew, domain.StateVisitScheduled, domain.StateClosed}

	for _, state := range tests {
		t.Run(string(state), func(t *testing.T) {
			l := mustNewLead(t)
			l.State = state

			err := l.ScheduleVisit(time.Now().Add(24 * time.Hour))
			if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Fatalf("got %v, want ErrInvalidTransition", err)
			}
			if l.VisitScheduledAt != nil {
				t.Fatal("VisitScheduledAt no debería setearse tras el error")
			}
		})
	}
}

// ─── Close ──────────────────────────────────────────────────────────────────

func TestClose_DesdeEstadosNoCerrados_Ok(t *testing.T) {
	tests := []domain.LeadState{domain.StateNew, domain.StateContacted, domain.StateVisitScheduled}

	for _, state := range tests {
		t.Run(string(state), func(t *testing.T) {
			l := mustNewLead(t)
			l.State = state
			originalUpdatedAt := l.UpdatedAt
			time.Sleep(time.Millisecond)

			if err := l.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if l.State != domain.StateClosed {
				t.Fatalf("state: got %s, want %s", l.State, domain.StateClosed)
			}
			if !l.UpdatedAt.After(originalUpdatedAt) {
				t.Fatalf("UpdatedAt debería haber avanzado: got %v, want > %v", l.UpdatedAt, originalUpdatedAt)
			}
		})
	}
}

func TestClose_DesdeClosed_RetornaErrInvalidTransition(t *testing.T) {
	l := mustNewLead(t)
	if err := l.Close(); err != nil {
		t.Fatalf("Close setup: %v", err)
	}
	updatedAtAlCerrar := l.UpdatedAt

	err := l.Close()

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
	if l.State != domain.StateClosed {
		t.Fatalf("el estado no debería cambiar tras el error: got %s", l.State)
	}
	if !l.UpdatedAt.Equal(updatedAtAlCerrar) {
		t.Fatalf("UpdatedAt no debería cambiar tras el error: got %v, want %v", l.UpdatedAt, updatedAtAlCerrar)
	}
}
