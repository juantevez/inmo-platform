package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
)

func TestScheduleVisitUseCase_ErrorAlBuscar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, dbErr },
	}
	uc := application.NewScheduleVisitUseCase(repo)

	_, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-1", VisitAt: time.Now().Add(24 * time.Hour)})
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestScheduleVisitUseCase_NoEncontrado_RetornaNotFound(t *testing.T) {
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil },
	}
	uc := application.NewScheduleVisitUseCase(repo)

	_, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-x", VisitAt: time.Now().Add(24 * time.Hour)})
	assertNotFound(t, err)
}

func TestScheduleVisitUseCase_EstadoNoPermiteAgendar_RetornaPreconditionFailed(t *testing.T) {
	l := newLeadFixture(t) // sigue en NEW, ScheduleVisit exige CONTACTED
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewScheduleVisitUseCase(repo)

	_, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-1", VisitAt: time.Now().Add(24 * time.Hour)})
	assertPreconditionFailed(t, err)
}

func TestScheduleVisitUseCase_FechaPasada_RetornaBadRequest(t *testing.T) {
	l := contactedLeadFixture(t)
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewScheduleVisitUseCase(repo)

	_, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-1", VisitAt: time.Now().Add(-1 * time.Hour)})
	assertBadRequest(t, err)
}

func TestScheduleVisitUseCase_Exitoso_AgendaYPublicaElEvento(t *testing.T) {
	l := contactedLeadFixture(t)
	visitAt := time.Now().Add(48 * time.Hour)
	var savedEventName string
	var savedPayload []byte
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			savedEventName = eventName
			savedPayload = eventPayload
			return nil
		},
	}
	uc := application.NewScheduleVisitUseCase(repo)

	dto, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-1", VisitAt: visitAt})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if l.State != domain.StateVisitScheduled {
		t.Fatalf("state del agregado: got %s, want %s", l.State, domain.StateVisitScheduled)
	}
	if dto.VisitScheduledAt == nil || *dto.VisitScheduledAt != visitAt.Format(time.RFC3339) {
		t.Fatalf("visit_scheduled_at en el DTO: %v", dto.VisitScheduledAt)
	}

	if savedEventName != "crm.lead.visit_scheduled" {
		t.Fatalf("event name: got %q, want %q", savedEventName, "crm.lead.visit_scheduled")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(savedPayload, &payload); err != nil {
		t.Fatalf("el payload del evento no es JSON válido: %v", err)
	}
	if payload["id"] != "lead-1" || payload["property_id"] != "prop-1" {
		t.Fatalf("payload del evento mapeado incorrectamente: %+v", payload)
	}
	if payload["visit_scheduled_at"] == nil {
		t.Fatal("el payload del evento debería incluir visit_scheduled_at")
	}
}

func TestScheduleVisitUseCase_ErrorAlGuardar_SePropagaTalCual(t *testing.T) {
	l := contactedLeadFixture(t)
	dbErr := errors.New("fallo de escritura")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			return dbErr
		},
	}
	uc := application.NewScheduleVisitUseCase(repo)

	_, err := uc.Execute(context.Background(), application.ScheduleVisitDTO{LeadID: "lead-1", VisitAt: time.Now().Add(24 * time.Hour)})
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}
