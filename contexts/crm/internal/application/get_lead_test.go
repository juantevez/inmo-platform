package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

var errFixtureNoConfigurada = errors.New("fixture no configurada")

// fakeLeadRepo implementa ports.LeadRepository.
type fakeLeadRepo struct {
	saveFn    func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error
	getByIDFn func(ctx context.Context, id string) (*domain.Lead, error)
}

func (f *fakeLeadRepo) Save(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, lead, eventName, eventPayload)
	}
	return errFixtureNoConfigurada
}

func (f *fakeLeadRepo) GetByID(ctx context.Context, id string) (*domain.Lead, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return nil, errFixtureNoConfigurada
}

func newLeadFixture(t *testing.T) *domain.Lead {
	t.Helper()
	l, err := domain.NewLead("lead-1", "prop-1", "Juan Pérez", "juan@test.com", "")
	if err != nil {
		t.Fatalf("NewLead: %v", err)
	}
	return l
}

func contactedLeadFixture(t *testing.T) *domain.Lead {
	t.Helper()
	l := newLeadFixture(t)
	if err := l.MarkContacted(); err != nil {
		t.Fatalf("MarkContacted setup: %v", err)
	}
	return l
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("got %v, want AppError NotFound", err)
	}
}

func assertPreconditionFailed(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestGetLeadUseCase_ErrorAlBuscar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, dbErr },
	}
	uc := application.NewGetLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestGetLeadUseCase_NoEncontrado_RetornaNotFound(t *testing.T) {
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil },
	}
	uc := application.NewGetLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-x")
	assertNotFound(t, err)
}

func TestGetLeadUseCase_Exitoso_MapeaElDTO(t *testing.T) {
	l := newLeadFixture(t)
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewGetLeadUseCase(repo)

	dto, err := uc.Execute(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if dto.ID != "lead-1" || dto.PropertyID != "prop-1" || dto.ClientName != "Juan Pérez" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", dto)
	}
	if dto.State != string(domain.StateNew) {
		t.Fatalf("state: got %s, want %s", dto.State, domain.StateNew)
	}
	if dto.VisitScheduledAt != nil {
		t.Fatalf("esperaba VisitScheduledAt nil, obtuve %v", dto.VisitScheduledAt)
	}
}

func TestGetLeadUseCase_ConVisitaAgendada_FormateaLaFechaComoRFC3339(t *testing.T) {
	l := contactedLeadFixture(t)
	visitAt := time.Now().Add(48 * time.Hour)
	if err := l.ScheduleVisit(visitAt); err != nil {
		t.Fatalf("ScheduleVisit setup: %v", err)
	}
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewGetLeadUseCase(repo)

	dto, err := uc.Execute(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if dto.VisitScheduledAt == nil || *dto.VisitScheduledAt != visitAt.Format(time.RFC3339) {
		t.Fatalf("visit_scheduled_at mapeado incorrectamente: %v", dto.VisitScheduledAt)
	}
}
