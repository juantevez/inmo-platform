package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inmo.platform/contexts/crm/internal/adapters/httpapi"
	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

var errFixtureNoConfigurada = errors.New("fixture no configurada")

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

func newHandler(repo *fakeLeadRepo) *httpapi.LeadHandler {
	return httpapi.NewLeadHandler(
		application.NewGetLeadUseCase(repo),
		application.NewContactLeadUseCase(repo),
		application.NewScheduleVisitUseCase(repo),
		application.NewCloseLeadUseCase(repo),
	)
}

func newRequestWithID(method, target, id string, body []byte) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.SetPathValue("id", id)
	return req
}

// ─── HandleGet ──────────────────────────────────────────────────────────────

func TestLeadHandler_HandleGet_NoEncontrado_Retorna404(t *testing.T) {
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodGet, "/api/v1/leads/lead-x", "lead-x", nil)
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLeadHandler_HandleGet_Exitoso_Retorna200(t *testing.T) {
	l := newLeadFixture(t)
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodGet, "/api/v1/leads/lead-1", "lead-1", nil)
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var dto application.LeadDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if dto.ID != "lead-1" {
		t.Fatalf("id: got %s, want lead-1", dto.ID)
	}
}

// ─── HandleContact ──────────────────────────────────────────────────────────

func TestLeadHandler_HandleContact_NoEncontrado_Retorna404(t *testing.T) {
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-x/contact", "lead-x", nil)
	rec := httptest.NewRecorder()
	h.HandleContact(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLeadHandler_HandleContact_EstadoInvalido_Retorna412(t *testing.T) {
	l := contactedLeadFixture(t) // ya CONTACTED
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/contact", "lead-1", nil)
	rec := httptest.NewRecorder()
	h.HandleContact(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestLeadHandler_HandleContact_Exitoso_Retorna200(t *testing.T) {
	l := newLeadFixture(t)
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn:    func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error { return nil },
	}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/contact", "lead-1", nil)
	rec := httptest.NewRecorder()
	h.HandleContact(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if l.State != domain.StateContacted {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateContacted)
	}
}

// ─── HandleScheduleVisit ────────────────────────────────────────────────────

func TestLeadHandler_HandleScheduleVisit_JSONInvalido_Retorna400(t *testing.T) {
	h := newHandler(&fakeLeadRepo{})

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", "lead-1", []byte("{invalido"))
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLeadHandler_HandleScheduleVisit_FechaFormatoInvalido_Retorna400(t *testing.T) {
	h := newHandler(&fakeLeadRepo{})

	body, _ := json.Marshal(map[string]string{"visit_at": "10-08-2026"})
	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", "lead-1", body)
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestLeadHandler_HandleScheduleVisit_NoEncontrado_Retorna404(t *testing.T) {
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil }}
	h := newHandler(repo)

	body, _ := json.Marshal(map[string]string{"visit_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339)})
	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-x/schedule-visit", "lead-x", body)
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLeadHandler_HandleScheduleVisit_FechaPasada_Retorna400(t *testing.T) {
	l := contactedLeadFixture(t)
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil }}
	h := newHandler(repo)

	body, _ := json.Marshal(map[string]string{"visit_at": time.Now().Add(-24 * time.Hour).Format(time.RFC3339)})
	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", "lead-1", body)
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestLeadHandler_HandleScheduleVisit_EstadoInvalido_Retorna412(t *testing.T) {
	l := newLeadFixture(t) // sigue en NEW
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil }}
	h := newHandler(repo)

	body, _ := json.Marshal(map[string]string{"visit_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339)})
	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", "lead-1", body)
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestLeadHandler_HandleScheduleVisit_Exitoso_Retorna200(t *testing.T) {
	l := contactedLeadFixture(t)
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn:    func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error { return nil },
	}
	h := newHandler(repo)

	visitAt := time.Now().Add(24 * time.Hour)
	body, _ := json.Marshal(map[string]string{"visit_at": visitAt.Format(time.RFC3339)})
	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/schedule-visit", "lead-1", body)
	rec := httptest.NewRecorder()
	h.HandleScheduleVisit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if l.State != domain.StateVisitScheduled {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateVisitScheduled)
	}
}

// ─── HandleClose ────────────────────────────────────────────────────────────

func TestLeadHandler_HandleClose_NoEncontrado_Retorna404(t *testing.T) {
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-x/close", "lead-x", nil)
	rec := httptest.NewRecorder()
	h.HandleClose(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLeadHandler_HandleClose_YaCerrado_Retorna412(t *testing.T) {
	l := newLeadFixture(t)
	if err := l.Close(); err != nil {
		t.Fatalf("Close setup: %v", err)
	}
	repo := &fakeLeadRepo{getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil }}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/close", "lead-1", nil)
	rec := httptest.NewRecorder()
	h.HandleClose(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestLeadHandler_HandleClose_Exitoso_Retorna200(t *testing.T) {
	l := newLeadFixture(t)
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn:    func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error { return nil },
	}
	h := newHandler(repo)

	req := newRequestWithID(http.MethodPost, "/api/v1/leads/lead-1/close", "lead-1", nil)
	rec := httptest.NewRecorder()
	h.HandleClose(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if l.State != domain.StateClosed {
		t.Fatalf("state: got %s, want %s", l.State, domain.StateClosed)
	}
}
