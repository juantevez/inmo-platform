package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inmo.platform/contexts/catalog/internal/adapters/httpapi"
	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeProfileRepo struct {
	byID        map[string]*domain.Profile
	byDniCuit   map[string]*domain.Profile
	findByIDErr error
	findDniErr  error
	saveErr     error
	saved       *domain.Profile
}

func (f *fakeProfileRepo) Save(ctx context.Context, profile *domain.Profile) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = profile
	return nil
}
func (f *fakeProfileRepo) FindByID(ctx context.Context, userID string) (*domain.Profile, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	if f.byID == nil {
		return nil, nil
	}
	return f.byID[userID], nil
}
func (f *fakeProfileRepo) FindByDniCuit(ctx context.Context, dniCuit string) (*domain.Profile, error) {
	if f.findDniErr != nil {
		return nil, f.findDniErr
	}
	if f.byDniCuit == nil {
		return nil, nil
	}
	return f.byDniCuit[dniCuit], nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newProfileHandler(repo *fakeProfileRepo) *httpapi.ProfileHandler {
	uc := application.NewCreateProfileUseCase(repo)
	return httpapi.NewProfileHandler(uc)
}

func decodeErrorField(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodeErrorField: %v, body=%q", err, rec.Body.String())
	}
	return got["error"]
}

// ─── HandleGetProfile ───────────────────────────────────────────────────────

func TestHandleGetProfile_SinXUserId_Retorna401(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/profiles/me", nil)
	rec := httptest.NewRecorder()

	h.HandleGetProfile(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetProfile_ErrorDelRepositorio_Retorna500(t *testing.T) {
	repo := &fakeProfileRepo{findByIDErr: errors.New("timeout de base de datos")}
	h := newProfileHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/profiles/me", nil)
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleGetProfile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleGetProfile_NoExiste_Retorna404(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/profiles/me", nil)
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleGetProfile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetProfile_HappyPath_Retorna200(t *testing.T) {
	profile, err := domain.NewProfile("user-1", "Juan", "Perez", "20-12345678-9", "+541112345678", domain.ProfileTypeIndividual, "", "")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	repo := &fakeProfileRepo{byID: map[string]*domain.Profile{"user-1": profile}}
	h := newProfileHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/profiles/me", nil)
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleGetProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got application.ProfileDTO
	decodeBody(t, rec, &got)
	if got.UserID != "user-1" || got.FirstName != "Juan" {
		t.Errorf("ProfileDTO: got %+v", got)
	}
}

// ─── HandleCreateOrUpdate ───────────────────────────────────────────────────

func TestHandleCreateOrUpdate_MetodoNoPermitido_Retorna405(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/profiles", nil)
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCreateOrUpdate_SinXUserId_Retorna401(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateOrUpdate_JSONInvalido_Retorna400(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBufferString("{invalido"))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateOrUpdate_CamposObligatoriosFaltantes_Retorna400(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	body, _ := json.Marshal(map[string]string{"profile_type": "INDIVIDUAL"}) // sin first_name/last_name/dni_cuit
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := decodeErrorField(t, rec); !strings.Contains(got, domain.ErrMissingRequiredFields.Error()) {
		t.Errorf("error: got %q, want que contenga %q", got, domain.ErrMissingRequiredFields.Error())
	}
}

func TestHandleCreateOrUpdate_TipoDePerfilInvalido_Retorna400(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	body, _ := json.Marshal(map[string]string{
		"first_name": "Juan", "last_name": "Perez", "dni_cuit": "20-12345678-9",
		"profile_type": "SUPERADMIN",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateOrUpdate_ComercialSinDatosDeEmpresa_Retorna400(t *testing.T) {
	h := newProfileHandler(&fakeProfileRepo{})
	body, _ := json.Marshal(map[string]string{
		"first_name": "Juan", "last_name": "Perez", "dni_cuit": "20-12345678-9",
		"profile_type": "COMMERCIAL", // sin company_name/license_number
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateOrUpdate_DniCuitYaRegistradoPorOtroUsuario_Retorna409(t *testing.T) {
	existing, err := domain.NewProfile("otro-user", "Ana", "Gomez", "20-99999999-9", "", domain.ProfileTypeIndividual, "", "")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	repo := &fakeProfileRepo{byDniCuit: map[string]*domain.Profile{"20-99999999-9": existing}}
	h := newProfileHandler(repo)

	body, _ := json.Marshal(map[string]string{
		"first_name": "Juan", "last_name": "Perez", "dni_cuit": "20-99999999-9",
		"profile_type": "INDIVIDUAL",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1") // distinto de "otro-user"
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleCreateOrUpdate_ErrorGenericoAlPersistir_Retorna500SinDetalle(t *testing.T) {
	repo := &fakeProfileRepo{saveErr: errors.New("fallo de escritura en Postgres con detalles sensibles")}
	h := newProfileHandler(repo)

	body, _ := json.Marshal(map[string]string{
		"first_name": "Juan", "last_name": "Perez", "dni_cuit": "20-12345678-9",
		"profile_type": "INDIVIDUAL",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	// El detalle interno del error NO debe filtrarse al cliente.
	got := decodeErrorField(t, rec)
	if strings.Contains(got, "Postgres") || strings.Contains(got, "detalles sensibles") {
		t.Errorf("error: got %q, no debería filtrar el detalle interno", got)
	}
}

func TestHandleCreateOrUpdate_HappyPath_Retorna201(t *testing.T) {
	repo := &fakeProfileRepo{}
	h := newProfileHandler(repo)

	body, _ := json.Marshal(map[string]string{
		"first_name": "Juan", "last_name": "Perez", "dni_cuit": "20-12345678-9",
		"phone": "+541112345678", "profile_type": "INDIVIDUAL",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/profiles", bytes.NewBuffer(body))
	req.Header.Set("X-User-Id", "user-1")
	rec := httptest.NewRecorder()

	h.HandleCreateOrUpdate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if repo.saved == nil || repo.saved.UserID() != "user-1" {
		t.Errorf("Save: got %+v, want el perfil persistido con UserID=user-1", repo.saved)
	}
}
