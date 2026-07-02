package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
	"inmo.platform/shared/pkg/ddd"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────
//
// PublishPropertyUseCase y UpdatePropertyUseCase corren su Save dentro de una
// transacción real de *sql.DB (con un type-assertion interno a una interfaz
// TxRepository que exige SaveWithTx). Por eso el fake implementa ese método
// además de ports.PropertyRepository, y usamos sqlmock para dar un *sql.DB
// que sepa responder a BeginTx/Commit sin una Postgres real.

type fakeRepo struct {
	mu         sync.Mutex
	properties map[string]*domain.Property

	findByIDFn    func(ctx context.Context, id string) (*domain.Property, error)
	findAllFn     func(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error)
	saveErr       error
	saveWithTxErr error
}

func (f *fakeRepo) Save(ctx context.Context, property *domain.Property) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.properties == nil {
		f.properties = make(map[string]*domain.Property)
	}
	f.properties[property.ID()] = property
	return nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (*domain.Property, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.properties[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (f *fakeRepo) FindAll(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
	if f.findAllFn != nil {
		return f.findAllFn(ctx, filters)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	results := make([]ports.PropertyResult, 0, len(f.properties))
	for _, p := range f.properties {
		results = append(results, ports.PropertyResult{Property: p})
	}
	return results, len(results), nil
}

// SaveWithTx satisface la interfaz privada TxRepository que exigen Publish/Update.
func (f *fakeRepo) SaveWithTx(ctx context.Context, tx *sql.Tx, p *domain.Property) error {
	if f.saveWithTxErr != nil {
		return f.saveWithTxErr
	}
	return f.Save(ctx, p)
}

func (f *fakeRepo) get(id string) *domain.Property {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.properties[id]
}

type fakeBlockedDatesRepo struct {
	hasOverlap bool
	overlapErr error
}

func (f *fakeBlockedDatesRepo) HasOverlap(ctx context.Context, propertyID string, start, end time.Time) (bool, error) {
	return f.hasOverlap, f.overlapErr
}
func (f *fakeBlockedDatesRepo) Block(ctx context.Context, propertyID, reservationID, reason string, start, end time.Time) error {
	return nil
}
func (f *fakeBlockedDatesRepo) Unblock(ctx context.Context, reservationID string) error { return nil }

// ─── Helpers ────────────────────────────────────────────────────────────────

// newMockDB da un *sql.DB respaldado por sqlmock, listo para BeginTx/Commit.
func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func newHandler(t *testing.T, repo *fakeRepo, blockedRepo ports.BlockedDatesRepository) (*httpapi.PropertyHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock := newMockDB(t)
	publishUC := application.NewPublishPropertyUseCase(db, repo)
	changeStateUC := application.NewChangePropertyStateUseCase(repo, realPublisher{})
	listUC := application.NewListPropertiesUseCase(repo)
	quoteUC := application.NewQuotePropertyUseCase(repo, blockedRepo)
	updateUC := application.NewUpdatePropertyUseCase(db, repo)

	h := httpapi.NewPropertyHandler(publishUC, changeStateUC, listUC, quoteUC, updateUC)
	return h, mock
}

// realPublisher: ports.EventPublisher no exige nada más que descartar los eventos.
type realPublisher struct{}

func (realPublisher) Publish(ctx context.Context, events ...ddd.DomainEvent) error { return nil }

// buildAvailableProperty crea una propiedad SALE válida y disponible, lista para
// sembrar directamente en el fakeRepo (bypaseando el HTTP layer).
func buildAvailableProperty(t *testing.T, id string) *domain.Property {
	t.Helper()
	price, err := domain.NewPrice(100000, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	loc, err := domain.NewLocation(-34.6, -58.4, "Av. Siempre Viva 742")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	p, err := domain.NewProperty(id, "owner-1", "Depto en Palermo", "Luminoso", price, loc, domain.OperationSale, domain.PetPolicyNotAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	return p
}

// buildTempProperty crea una propiedad TEMP con TempConfig válido, para los tests de Quote.
func buildTempProperty(t *testing.T, id string) *domain.Property {
	t.Helper()
	price, err := domain.NewPrice(100, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	loc, err := domain.NewLocation(-34.6, -58.4, "Av. Siempre Viva 742")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	p, err := domain.NewProperty(id, "owner-1", "Depto temporario", "Con vista al mar", price, loc, domain.OperationTemp, domain.PetPolicyAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	tc, err := domain.NewTempConfig(nil, "14:00", "10:00", 2, 30, 50, 20, 100, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	p.SetTempConfig(tc)
	return p
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decodeBody: %v, body=%q", err, rec.Body.String())
	}
}

// ─── Publish ────────────────────────────────────────────────────────────────

func TestPublish_SinXUserId_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()

	h.Publish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublish_JSONInvalido_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()

	h.Publish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublish_ErrorDeValidacionDeDominio_Retorna400(t *testing.T) {
	// Título vacío es rechazado por domain.NewProperty antes de tocar la DB —
	// no hace falta configurar expectativas de sqlmock.
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	body, _ := json.Marshal(map[string]interface{}{
		"title": "", "price": 1000, "currency": "USD",
		"latitude": -34.6, "longitude": -58.4, "address": "Calle Falsa 123",
		"operation_type": "SALE", "pet_policy": "NOT_ALLOWED",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()

	h.Publish(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPublish_HappyPath_Retorna201YGeneraIDSiNoViene(t *testing.T) {
	repo := &fakeRepo{}
	h, mock := newHandler(t, repo, &fakeBlockedDatesRepo{})
	mock.ExpectBegin()
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]interface{}{
		// sin "id" — el handler debe generar uno con uuid.New()
		"title": "Depto en Belgrano", "price": 250000, "currency": "USD",
		"latitude": -34.6, "longitude": -58.4, "address": "Calle Falsa 123",
		"operation_type": "SALE", "pet_policy": "NOT_ALLOWED",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()

	h.Publish(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(repo.properties) != 1 {
		t.Fatalf("propiedades guardadas: got %d, want 1", len(repo.properties))
	}
	for id, p := range repo.properties {
		if id == "" || p.OwnerID() != "owner-1" {
			t.Errorf("propiedad guardada: got ID=%q OwnerID=%q", id, p.OwnerID())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestPublish_ErrorGenericoDeInfraestructura_Retorna500SinAppError(t *testing.T) {
	// BeginTx fallando devuelve un error crudo (no un *apperr.AppError) — el
	// handler debe caer al branch genérico 500 de errorResponse.
	repo := &fakeRepo{}
	h, mock := newHandler(t, repo, &fakeBlockedDatesRepo{})
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	body, _ := json.Marshal(map[string]interface{}{
		"id": "prop-1", "title": "Depto", "price": 1000, "currency": "USD",
		"latitude": -34.6, "longitude": -58.4, "address": "Calle Falsa 123",
		"operation_type": "SALE", "pet_policy": "NOT_ALLOWED",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()

	h.Publish(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "un error inesperado ocurrió") {
		t.Errorf("body: got %q, want el mensaje genérico de error interno", rec.Body.String())
	}
}

// ─── Reserve ────────────────────────────────────────────────────────────────

func TestReserve_SinIDEnPath_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties//reserve", nil)
	rec := httptest.NewRecorder()

	h.Reserve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReserve_PropiedadNoExiste_Retorna404(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/reserve", nil)
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.Reserve(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestReserve_HappyPath_CambiaEstadoARetorna200(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	prop := buildAvailableProperty(t, "prop-1")
	repo.properties["prop-1"] = prop
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/reserve", nil)
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Reserve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if repo.get("prop-1").State() != domain.StateReserved {
		t.Errorf("State tras Reserve: got %q, want %q", repo.get("prop-1").State(), domain.StateReserved)
	}
}

func TestReserve_PropiedadYaReservada_Retorna412(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	prop := buildAvailableProperty(t, "prop-1")
	if err := prop.Reserve(); err != nil {
		t.Fatalf("Reserve setup: %v", err)
	}
	repo.properties["prop-1"] = prop
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/reserve", nil)
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Reserve(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusPreconditionFailed)
	}
}

// ─── List ───────────────────────────────────────────────────────────────────

func TestList_SinQueryParams_UsaDefaults(t *testing.T) {
	var captured ports.ListFilters
	repo := &fakeRepo{
		findAllFn: func(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
			captured = f
			return nil, 0, nil
		},
	}
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if captured.Limit != 50 || captured.Offset != 0 {
		t.Errorf("filtros default: got Limit=%d Offset=%d, want Limit=50 Offset=0", captured.Limit, captured.Offset)
	}
}

func TestList_ConQueryParams_ParseaFiltrosCorrectamente(t *testing.T) {
	var captured ports.ListFilters
	repo := &fakeRepo{
		findAllFn: func(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
			captured = f
			return nil, 0, nil
		},
	}
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties?status=AVAILABLE&operation=SALE&pets=ALLOWED&owner_id=owner-1&limit=10&offset=5&min_price=100&max_price=500&lat=-34.6&lon=-58.4&radius_km=5", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	want := ports.ListFilters{
		State: "AVAILABLE", OperationType: "SALE", PetPolicy: "ALLOWED", OwnerID: "owner-1",
		Limit: 10, Offset: 5, MinPrice: 100, MaxPrice: 500,
		Latitude: -34.6, Longitude: -58.4, RadiusKm: 5,
	}
	if captured != want {
		t.Errorf("filtros: got %+v, want %+v", captured, want)
	}
}

func TestList_LimiteInvalidoONegativo_CaeAlDefault(t *testing.T) {
	cases := []string{"abc", "-5"}
	for _, limitParam := range cases {
		t.Run(limitParam, func(t *testing.T) {
			var captured ports.ListFilters
			repo := &fakeRepo{
				findAllFn: func(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
					captured = f
					return nil, 0, nil
				},
			}
			h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/properties?limit="+limitParam, nil)
			rec := httptest.NewRecorder()

			h.List(rec, req)

			if captured.Limit != 50 {
				t.Errorf("Limit con param %q: got %d, want 50 (default)", limitParam, captured.Limit)
			}
		})
	}
}

func TestList_HappyPath_DevuelveListResponse(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var resp application.ListResponse
	decodeBody(t, rec, &resp)
	if resp.Total != 1 || len(resp.Properties) != 1 {
		t.Fatalf("ListResponse: got %+v, want 1 propiedad", resp)
	}
	if resp.Properties[0].ID != "prop-1" {
		t.Errorf("Properties[0].ID: got %q, want %q", resp.Properties[0].ID, "prop-1")
	}
}

func TestList_ErrorDelRepositorio_Retorna500(t *testing.T) {
	repo := &fakeRepo{
		findAllFn: func(ctx context.Context, f ports.ListFilters) ([]ports.PropertyResult, int, error) {
			return nil, 0, errors.New("timeout de base de datos")
		},
	}
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/properties", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ─── Quote ──────────────────────────────────────────────────────────────────

func TestQuote_JSONInvalido_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/quote", bytes.NewBufferString("{invalido"))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQuote_FechaCheckInInvalida_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	body, _ := json.Marshal(map[string]string{"check_in_date": "31/12/2024", "check_out_date": "2025-01-05"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/quote", bytes.NewBuffer(body))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "check_in_date") {
		t.Errorf("body: got %q, want que mencione check_in_date", rec.Body.String())
	}
}

func TestQuote_FechaCheckOutInvalida_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	body, _ := json.Marshal(map[string]string{"check_in_date": "2025-01-01", "check_out_date": "31/12/2025"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/quote", bytes.NewBuffer(body))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestQuote_PropiedadNoExiste_Retorna404(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	body, _ := json.Marshal(map[string]string{"check_in_date": "2025-01-01", "check_out_date": "2025-01-05"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/quote", bytes.NewBuffer(body))
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestQuote_FechasSuperpuestas_Retorna412(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildTempProperty(t, "prop-1")
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{hasOverlap: true})

	body, _ := json.Marshal(map[string]string{"check_in_date": "2025-01-01", "check_out_date": "2025-01-05"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/quote", bytes.NewBuffer(body))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestQuote_HappyPath_CalculaCotizacion(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildTempProperty(t, "prop-1")
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{hasOverlap: false})

	body, _ := json.Marshal(map[string]string{"check_in_date": "2025-01-01", "check_out_date": "2025-01-05"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/prop-1/quote", bytes.NewBuffer(body))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Quote(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp application.QuoteResponse
	decodeBody(t, rec, &resp)
	// 4 noches (01 al 05) a 50/noche = 200, + cleaning fee 20 = 220
	if resp.Nights != 4 {
		t.Errorf("Nights: got %d, want 4", resp.Nights)
	}
	if resp.Total != 220 {
		t.Errorf("Total: got %v, want 220", resp.Total)
	}
}

// ─── Update ─────────────────────────────────────────────────────────────────

func TestUpdate_SinXUserId_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/prop-1", bytes.NewBufferString("{}"))
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdate_SinIDEnPath_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/", bytes.NewBufferString("{}"))
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdate_JSONInvalido_Retorna400(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/prop-1", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdate_PropiedadNoExiste_Retorna404(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	body, _ := json.Marshal(map[string]string{"title": "Nuevo título"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/no-existe", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdate_HappyPath_ActualizaYRetorna200(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	repo.properties["prop-1"] = buildAvailableProperty(t, "prop-1")
	h, mock := newHandler(t, repo, &fakeBlockedDatesRepo{})
	mock.ExpectBegin()
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]interface{}{"title": "Depto renovado", "price": 200000})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/prop-1", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated := repo.get("prop-1")
	if updated.Title() != "Depto renovado" || updated.Price().Amount() != 200000 {
		t.Errorf("propiedad actualizada: got Title=%q Price=%v", updated.Title(), updated.Price().Amount())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestUpdate_PropiedadReservada_Retorna412(t *testing.T) {
	repo := &fakeRepo{properties: map[string]*domain.Property{}}
	prop := buildAvailableProperty(t, "prop-1")
	if err := prop.Reserve(); err != nil {
		t.Fatalf("Reserve setup: %v", err)
	}
	repo.properties["prop-1"] = prop
	h, _ := newHandler(t, repo, &fakeBlockedDatesRepo{})

	body, _ := json.Marshal(map[string]interface{}{"title": "Nuevo título"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/properties/prop-1", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "owner-1")
	req.SetPathValue("id", "prop-1")
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

// ─── errorResponse: estructura del AppError expuesta al cliente ───────────────

func TestErrorResponse_AppError_ExponeTypeYMessage(t *testing.T) {
	h, _ := newHandler(t, &fakeRepo{}, &fakeBlockedDatesRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/properties/no-existe/reserve", nil)
	req.SetPathValue("id", "no-existe")
	rec := httptest.NewRecorder()

	h.Reserve(rec, req)

	var got apperr.AppError
	decodeBody(t, rec, &got)
	if got.Type != apperr.TypeNotFound {
		t.Errorf("Type: got %q, want %q", got.Type, apperr.TypeNotFound)
	}
	if got.Message == "" {
		t.Error("Message: no debería estar vacío")
	}
}
