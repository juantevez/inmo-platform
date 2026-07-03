package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/adapters/httpapi"
	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeReservationRepo struct {
	saveFn                           func(ctx context.Context, r *domain.Reservation) error
	saveWithTxFn                     func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error
	findByIDFn                       func(ctx context.Context, id string) (*domain.Reservation, error)
	hasOverlapFn                     func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error)
	findByOwnerIDFn                  func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error)
	findConfirmedCheckingInBetweenFn func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error)
	markReminderSentFn               func(ctx context.Context, reservationID string) error
}

func (f *fakeReservationRepo) Save(ctx context.Context, r *domain.Reservation) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, r)
	}
	return errFixtureNoConfigurada
}
func (f *fakeReservationRepo) SaveWithTx(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error {
	if f.saveWithTxFn != nil {
		return f.saveWithTxFn(ctx, tx, r)
	}
	return errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindByID(ctx context.Context, id string) (*domain.Reservation, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) HasOverlap(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
	if f.hasOverlapFn != nil {
		return f.hasOverlapFn(ctx, propertyID, checkIn, checkOut)
	}
	return false, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindByOwnerID(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
	if f.findByOwnerIDFn != nil {
		return f.findByOwnerIDFn(ctx, ownerID, statusFilter)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) FindConfirmedCheckingInBetween(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
	if f.findConfirmedCheckingInBetweenFn != nil {
		return f.findConfirmedCheckingInBetweenFn(ctx, from, to)
	}
	return nil, errFixtureNoConfigurada
}
func (f *fakeReservationRepo) MarkReminderSent(ctx context.Context, reservationID string) error {
	if f.markReminderSentFn != nil {
		return f.markReminderSentFn(ctx, reservationID)
	}
	return errFixtureNoConfigurada
}

type fakeSnapshotRepo struct {
	upsertFn   func(ctx context.Context, snap domain.PropertySnapshot) error
	findByIDFn func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error)
}

func (f *fakeSnapshotRepo) Upsert(ctx context.Context, snap domain.PropertySnapshot) error {
	if f.upsertFn != nil {
		return f.upsertFn(ctx, snap)
	}
	return errFixtureNoConfigurada
}
func (f *fakeSnapshotRepo) FindByID(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, propertyID)
	}
	return nil, errFixtureNoConfigurada
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func validSnapshot() *domain.PropertySnapshot {
	return &domain.PropertySnapshot{
		PropertyID:      "prop-1",
		OwnerID:         "owner-1",
		OperationType:   "TEMPORARY",
		NightPrice:      10000,
		CleaningFee:     1500,
		SecurityDeposit: 5000,
		MinNights:       2,
		MaxNights:       30,
		CheckInTime:     "14:00",
		CheckOutTime:    "10:00",
	}
}

func pendingReservation(t *testing.T) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	checkOut := checkIn.Add(3 * 24 * time.Hour)
	r, err := domain.NewReservation("res-1", "prop-1", "tenant-1", "owner-1",
		checkIn, checkOut, 3, 10000, 0, 1500, 5000, 31500, "")
	if err != nil {
		t.Fatalf("NewReservation: %v", err)
	}
	r.PullEvents()
	return r
}

func newReservationRequest(t *testing.T, method, target, userID string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}
	return req
}

// ─── HandleCreate ───────────────────────────────────────────────────────────

func TestReservationHandler_HandleCreate_SinXUserId_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, &fakeSnapshotRepo{}),
		nil, nil, nil, nil,
	)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "", []byte(`{}`))
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReservationHandler_HandleCreate_JSONInvalido_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, &fakeSnapshotRepo{}),
		nil, nil, nil, nil,
	)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", []byte("{invalido"))
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReservationHandler_HandleCreate_FechaInvalida_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, &fakeSnapshotRepo{}),
		nil, nil, nil, nil,
	)

	tests := []struct {
		name string
		body string
	}{
		{"check_in_date inválida", `{"property_id":"prop-1","check_in_date":"31-12-2026","check_out_date":"2026-08-15"}`},
		{"check_out_date inválida", `{"property_id":"prop-1","check_in_date":"2026-08-10","check_out_date":"no-es-fecha"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", []byte(tc.body))
			rec := httptest.NewRecorder()
			h.HandleCreate(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func createReservationBody(checkIn, checkOut time.Time) []byte {
	body, _ := json.Marshal(map[string]any{
		"property_id":    "prop-1",
		"check_in_date":  checkIn.Format("2006-01-02"),
		"check_out_date": checkOut.Format("2006-01-02"),
		"guest_message":  "Llegamos tarde",
	})
	return body
}

func TestReservationHandler_HandleCreate_PropiedadNoEncontrada_Retorna404(t *testing.T) {
	db, _ := newMockDB(t)
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return nil, nil }}
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo),
		nil, nil, nil, nil,
	)

	checkIn := time.Now().Add(24 * time.Hour)
	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", createReservationBody(checkIn, checkIn.Add(3*24*time.Hour)))
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestReservationHandler_HandleCreate_EstadiaFueraDeRango_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	snap := validSnapshot()
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil }}
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, &fakeReservationRepo{}, snapRepo),
		nil, nil, nil, nil,
	)

	tests := []struct {
		name   string
		nights int
	}{
		{"menos que el mínimo", 1},
		{"más que el máximo", 31},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkIn := time.Now().Add(24 * time.Hour)
			checkOut := checkIn.Add(time.Duration(tc.nights) * 24 * time.Hour)
			req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", createReservationBody(checkIn, checkOut))
			rec := httptest.NewRecorder()
			h.HandleCreate(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestReservationHandler_HandleCreate_FechasSuperpuestas_Retorna412(t *testing.T) {
	db, _ := newMockDB(t)
	snap := validSnapshot()
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil }}
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return true, nil
		},
	}
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, resRepo, snapRepo),
		nil, nil, nil, nil,
	)

	checkIn := time.Now().Add(24 * time.Hour)
	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", createReservationBody(checkIn, checkIn.Add(3*24*time.Hour)))
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestReservationHandler_HandleCreate_Exitoso_Retorna201(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	snap := validSnapshot()
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil }}
	var saved *domain.Reservation
	resRepo := &fakeReservationRepo{
		hasOverlapFn: func(ctx context.Context, propertyID string, checkIn, checkOut time.Time) (bool, error) {
			return false, nil
		},
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, r *domain.Reservation) error { saved = r; return nil },
	}
	h := httpapi.NewReservationHandler(
		application.NewCreateReservationUseCase(db, resRepo, snapRepo),
		nil, nil, nil, nil,
	)

	checkIn := time.Now().Add(24 * time.Hour)
	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations", "tenant-1", createReservationBody(checkIn, checkIn.Add(3*24*time.Hour)))
	rec := httptest.NewRecorder()
	h.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if saved == nil || saved.PropertyID() != "prop-1" || saved.TenantID() != "tenant-1" || saved.OwnerID() != "owner-1" {
		t.Fatalf("reserva guardada incorrectamente: %+v", saved)
	}

	var dto application.ReservationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if dto.Status != string(domain.ReservationPendingApproval) {
		t.Fatalf("status en el DTO: got %s", dto.Status)
	}
	if dto.CheckInTime != "14:00" || dto.CheckOutTime != "10:00" {
		t.Fatalf("horarios del snapshot no propagados al DTO: %+v", dto)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ─── HandleConfirm ──────────────────────────────────────────────────────────

func TestReservationHandler_HandleConfirm_SinXUserId_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	h := httpapi.NewReservationHandler(nil,
		application.NewConfirmReservationUseCase(db, &fakeReservationRepo{}, &fakeSnapshotRepo{}), nil, nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/confirm", "", nil)
	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReservationHandler_HandleConfirm_NoEncontrada_Retorna404(t *testing.T) {
	db, _ := newMockDB(t)
	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, nil }}
	h := httpapi.NewReservationHandler(nil,
		application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{}), nil, nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/confirm", "owner-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestReservationHandler_HandleConfirm_NoEsElPropietario_Retorna403(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservation(t)
	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil }}
	h := httpapi.NewReservationHandler(nil,
		application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{}), nil, nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/confirm", "otro-usuario", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestReservationHandler_HandleConfirm_Exitoso_Retorna200(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservation(t)
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
		return validSnapshot(), nil
	}}
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	h := httpapi.NewReservationHandler(nil,
		application.NewConfirmReservationUseCase(db, resRepo, snapRepo), nil, nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/confirm", "owner-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if r.Status() != domain.ReservationConfirmed {
		t.Fatalf("la reserva no quedó confirmada: %s", r.Status())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationHandler_HandleConfirm_EstadoNoPermiteConfirmar_Retorna412(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservation(t)
	if err := r.Confirm(); err != nil {
		t.Fatalf("Confirm setup: %v", err)
	}
	r.PullEvents()

	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil }}
	h := httpapi.NewReservationHandler(nil,
		application.NewConfirmReservationUseCase(db, resRepo, &fakeSnapshotRepo{}), nil, nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/confirm", "owner-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleConfirm(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

// ─── HandleCancel ───────────────────────────────────────────────────────────

func TestReservationHandler_HandleCancel_SinXUserId_Retorna400(t *testing.T) {
	db, _ := newMockDB(t)
	h := httpapi.NewReservationHandler(nil, nil,
		application.NewCancelReservationUseCase(db, &fakeReservationRepo{}), nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/cancel", "", nil)
	rec := httptest.NewRecorder()
	h.HandleCancel(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReservationHandler_HandleCancel_NiTenantNiOwner_Retorna403(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservation(t)
	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil }}
	h := httpapi.NewReservationHandler(nil, nil,
		application.NewCancelReservationUseCase(db, resRepo), nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/cancel", "intruso", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleCancel(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestReservationHandler_HandleCancel_Exitoso_Retorna200(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	r := pendingReservation(t)
	resRepo := &fakeReservationRepo{
		findByIDFn:   func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil },
		saveWithTxFn: func(ctx context.Context, tx *sql.Tx, res *domain.Reservation) error { return nil },
	}
	h := httpapi.NewReservationHandler(nil, nil,
		application.NewCancelReservationUseCase(db, resRepo), nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/cancel", "tenant-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleCancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if r.Status() != domain.ReservationCancelled {
		t.Fatalf("la reserva no quedó cancelada: %s", r.Status())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReservationHandler_HandleCancel_YaFinalizada_Retorna412(t *testing.T) {
	db, _ := newMockDB(t)
	r := pendingReservation(t)
	if err := r.Cancel(); err != nil {
		t.Fatalf("Cancel setup: %v", err)
	}
	r.PullEvents()

	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil }}
	h := httpapi.NewReservationHandler(nil, nil,
		application.NewCancelReservationUseCase(db, resRepo), nil, nil)

	req := newReservationRequest(t, http.MethodPost, "/api/v1/reservations/res-1/cancel", "tenant-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleCancel(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

// ─── HandleGet ──────────────────────────────────────────────────────────────

func TestReservationHandler_HandleGet_NoEncontrada_Retorna404(t *testing.T) {
	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return nil, nil }}
	h := httpapi.NewReservationHandler(nil, nil, nil,
		application.NewGetReservationUseCase(resRepo, &fakeSnapshotRepo{}), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/res-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestReservationHandler_HandleGet_Exitoso_Retorna200(t *testing.T) {
	r := pendingReservation(t)
	snap := validSnapshot()
	resRepo := &fakeReservationRepo{findByIDFn: func(ctx context.Context, id string) (*domain.Reservation, error) { return r, nil }}
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) { return snap, nil }}
	h := httpapi.NewReservationHandler(nil, nil, nil,
		application.NewGetReservationUseCase(resRepo, snapRepo), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/res-1", nil)
	req.SetPathValue("id", "res-1")
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var dto application.ReservationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if dto.ID != "res-1" {
		t.Fatalf("id: got %s, want res-1", dto.ID)
	}
}

// ─── HandleListOwner ────────────────────────────────────────────────────────

func TestReservationHandler_HandleListOwner_SinXUserId_Retorna400(t *testing.T) {
	h := httpapi.NewReservationHandler(nil, nil, nil, nil,
		application.NewGetOwnerReservationsUseCase(&fakeReservationRepo{}, &fakeSnapshotRepo{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/owner", nil)
	rec := httptest.NewRecorder()
	h.HandleListOwner(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestReservationHandler_HandleListOwner_Exitoso_Retorna200(t *testing.T) {
	r := pendingReservation(t)
	var capturedStatusFilter string
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			capturedStatusFilter = statusFilter
			return []*domain.Reservation{r}, nil
		},
	}
	snapRepo := &fakeSnapshotRepo{findByIDFn: func(ctx context.Context, propertyID string) (*domain.PropertySnapshot, error) {
		return validSnapshot(), nil
	}}
	h := httpapi.NewReservationHandler(nil, nil, nil, nil,
		application.NewGetOwnerReservationsUseCase(resRepo, snapRepo))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/owner?status=PENDING_APPROVAL", nil)
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()
	h.HandleListOwner(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if capturedStatusFilter != "PENDING_APPROVAL" {
		t.Fatalf("status filter no propagado: got %q", capturedStatusFilter)
	}
	var dtos []application.ReservationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dtos); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(dtos) != 1 || dtos[0].ID != "res-1" {
		t.Fatalf("dtos: %+v", dtos)
	}
}

func TestReservationHandler_HandleListOwner_ErrorDelRepo_PropagaError(t *testing.T) {
	resRepo := &fakeReservationRepo{
		findByOwnerIDFn: func(ctx context.Context, ownerID, statusFilter string) ([]*domain.Reservation, error) {
			return nil, errors.New("fallo de conexión")
		},
	}
	h := httpapi.NewReservationHandler(nil, nil, nil, nil,
		application.NewGetOwnerReservationsUseCase(resRepo, &fakeSnapshotRepo{}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reservations/owner", nil)
	req.Header.Set("X-User-Id", "owner-1")
	rec := httptest.NewRecorder()
	h.HandleListOwner(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
