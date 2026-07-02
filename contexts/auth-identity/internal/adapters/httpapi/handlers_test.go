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

	"inmo.platform/contexts/auth-identity/internal/adapters/httpapi"
	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

// ─── Fakes de los puertos ──────────────────────────────────────────────────────
//
// A diferencia de los tests del paquete application (que verifican la lógica de
// negocio con precisión), estos fakes priorizan comportamiento "feliz" por
// defecto: lo que le interesa a este archivo es el mapeo HTTP (status codes,
// decodificación de JSON, armado del comando), no repetir la cobertura de los
// casos de uso.

type fakeUserRepo struct {
	findByEmailFn           func(ctx context.Context, email string) (*domain.User, error)
	findProviderFn          func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error)
	findByIDFn              func(ctx context.Context, id string) (*domain.User, error)
	findVerificationTokenFn func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error)
	saveFn                  func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error
}

func (f *fakeUserRepo) Save(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, user, provider, roles)
	}
	return nil
}
func (f *fakeUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, nil
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.findByEmailFn != nil {
		return f.findByEmailFn(ctx, email)
	}
	return nil, nil
}
func (f *fakeUserRepo) FindRolesByUserID(ctx context.Context, userID string) ([]string, error) {
	return []string{"INQUILINO"}, nil
}
func (f *fakeUserRepo) FindProvider(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
	if f.findProviderFn != nil {
		return f.findProviderFn(ctx, userID, pType)
	}
	return nil, nil
}
func (f *fakeUserRepo) FindByProviderKey(ctx context.Context, pType domain.ProviderType, providerUserID string) (*domain.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) AddProvider(ctx context.Context, userID string, provider *domain.IdentityProvider) error {
	return nil
}
func (f *fakeUserRepo) SaveVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	return nil
}
func (f *fakeUserRepo) FindVerificationToken(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
	if f.findVerificationTokenFn != nil {
		return f.findVerificationTokenFn(ctx, tokenValue, tType)
	}
	return nil, nil
}
func (f *fakeUserRepo) UpdateVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	return nil
}

type fakeTokenRepo struct {
	incrementFn func(ctx context.Context, key string, window time.Duration) (int, error)
}

func (f *fakeTokenRepo) SetRefreshToken(ctx context.Context, tokenID string, userID string, ttl time.Duration) error {
	return nil
}
func (f *fakeTokenRepo) GetRefreshToken(ctx context.Context, tokenID string) (string, error) {
	return "", nil
}
func (f *fakeTokenRepo) DeleteRefreshToken(ctx context.Context, tokenID string) error { return nil }
func (f *fakeTokenRepo) DeleteAllRefreshTokens(ctx context.Context, userID string) error {
	return nil
}
func (f *fakeTokenRepo) AddToBlocklist(ctx context.Context, tokenStr string, ttl time.Duration) error {
	return nil
}
func (f *fakeTokenRepo) IsInBlocklist(ctx context.Context, tokenStr string) (bool, error) {
	return false, nil
}
func (f *fakeTokenRepo) IncrementLoginAttempts(ctx context.Context, key string, window time.Duration) (int, error) {
	if f.incrementFn != nil {
		return f.incrementFn(ctx, key, window)
	}
	return 1, nil
}
func (f *fakeTokenRepo) ClearLoginAttempts(ctx context.Context, key string) error { return nil }

type fakeEventPublisher struct{}

func (f *fakeEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	return nil
}

type fakeTokenService struct{}

func (f *fakeTokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	return "access-token", nil
}
func (f *fakeTokenService) GenerateRefreshToken() (string, error) {
	return "refresh-token", nil
}

type fakeIdentityService struct {
	verifyGoogleFn func(ctx context.Context, code string) (*ports.SSOResult, error)
	verifyMetaFn   func(ctx context.Context, accessToken string) (*ports.SSOResult, error)
}

func (f *fakeIdentityService) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	if f.verifyGoogleFn != nil {
		return f.verifyGoogleFn(ctx, code)
	}
	return &ports.SSOResult{Email: "nuevo@gmail.com", ProviderUserID: "google-sub-1"}, nil
}
func (f *fakeIdentityService) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	if f.verifyMetaFn != nil {
		return f.verifyMetaFn(ctx, accessToken)
	}
	return &ports.SSOResult{Email: "nuevo@fb.com", ProviderUserID: "meta-id-1"}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func fixedUUIDGen(id string) application.UUIDGenerator {
	return func() string { return id }
}

// testEnv agrupa los fakes compartidos por los 5 casos de uso que arma el handler.
type testEnv struct {
	userRepo    *fakeUserRepo
	tokenRepo   *fakeTokenRepo
	identitySvc *fakeIdentityService
	tokenSvc    *fakeTokenService
	publisher   *fakeEventPublisher
}

func newTestHandler(env testEnv) *httpapi.AuthHandler {
	uuidGen := fixedUUIDGen("id-1")
	registerUC := application.NewRegisterUserUseCase(env.userRepo, env.publisher, uuidGen)
	loginPassUC := application.NewLoginPasswordUseCase(env.userRepo, env.tokenRepo, env.tokenSvc, env.publisher, uuidGen)
	verifyEmailUC := application.NewVerifyEmailUseCase(env.userRepo, env.tokenRepo, env.tokenSvc, env.publisher, uuidGen)
	loginGoogleUC := application.NewLoginSSOGoogleUseCase(env.userRepo, env.tokenRepo, env.identitySvc, env.tokenSvc, env.publisher, uuidGen)
	loginMetaUC := application.NewLoginSSOMetaUseCase(env.userRepo, env.tokenRepo, env.identitySvc, env.tokenSvc, env.publisher, uuidGen)

	ssoConfig := httpapi.SSOPublicConfig{
		GoogleClientID:    "google-client-id",
		GoogleRedirectURI: "https://app.test/callback",
		MetaAppID:         "meta-app-id",
	}

	return httpapi.NewAuthHandler(registerUC, loginPassUC, verifyEmailUC, loginGoogleUC, loginMetaUC, ssoConfig)
}

func newDefaultEnv() testEnv {
	return testEnv{
		userRepo:    &fakeUserRepo{},
		tokenRepo:   &fakeTokenRepo{},
		identitySvc: &fakeIdentityService{},
		tokenSvc:    &fakeTokenService{},
		publisher:   &fakeEventPublisher{},
	}
}

func decodeJSON(t *testing.T, body *bytes.Buffer, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		t.Fatalf("decodeJSON: error decodificando respuesta: %v, body=%q", err, body.String())
	}
}

// ─── HandleRegister ─────────────────────────────────────────────────────────

func TestHandleRegister_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString("{invalido"))
	rec := httptest.NewRecorder()

	h.HandleRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRegister_RolInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"email": "nuevo@test.com", "password": "Password1", "role": "SUPERADMIN"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var got map[string]string
	decodeJSON(t, rec.Body, &got)
	if got["error"] != application.ErrInvalidRole.Error() {
		t.Errorf("error: got %q, want %q", got["error"], application.ErrInvalidRole.Error())
	}
}

func TestHandleRegister_EmailYaExiste_Retorna409(t *testing.T) {
	env := newDefaultEnv()
	existingUser, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	existingProvider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return existingUser, nil }
	env.userRepo.findProviderFn = func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
		return existingProvider, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1", "role": "INQUILINO"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleRegister(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleRegister_ErrorGenerico_Retorna422(t *testing.T) {
	// Una contraseña débil hace fallar la construcción del agregado en el dominio;
	// el handler no la reconoce como un error tipado y cae al branch genérico 422.
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"email": "nuevo@test.com", "password": "corta", "role": "INQUILINO"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleRegister(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegister_HappyPath_Retorna201(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"email": "nuevo@test.com", "password": "Password1", "role": "PROPIETARIO"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got application.RegisterUserResponse
	decodeJSON(t, rec.Body, &got)
	if got.UserID != "id-1" || got.Role != "PROPIETARIO" {
		t.Errorf("body: got %+v, want UserID=id-1 Role=PROPIETARIO", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// ─── HandleLoginPassword ────────────────────────────────────────────────────

func TestHandleLoginPassword_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("{invalido"))
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleLoginPassword_RateLimitExcedido_Retorna429(t *testing.T) {
	env := newDefaultEnv()
	env.tokenRepo.incrementFn = func(ctx context.Context, key string, window time.Duration) (int, error) { return 6, nil }
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestHandleLoginPassword_CredencialesInvalidas_Retorna401(t *testing.T) {
	// FindByEmail por defecto devuelve nil (usuario inexistente) -> ErrInvalidCredentials
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"email": "nadie@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleLoginPassword_EmailNoVerificado_Retorna403(t *testing.T) {
	env := newDefaultEnv()
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return pending, nil }
	env.userRepo.findProviderFn = func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
		return provider, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleLoginPassword_ErrorGenerico_Retorna500SinFiltrarDetalle(t *testing.T) {
	// Una cuenta suspendida dispara un error de dominio no tipado (errors.New plano);
	// el handler debe caer al branch genérico 500 SIN filtrar el mensaje interno.
	env := newDefaultEnv()
	suspended := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return suspended, nil }
	env.userRepo.findProviderFn = func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
		return provider, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var got map[string]string
	decodeJSON(t, rec.Body, &got)
	if got["error"] != "Internal Server Error" {
		t.Errorf("error: got %q, want el mensaje genérico sin detalles internos", got["error"])
	}
}

func TestHandleLoginPassword_HappyPath_Retorna200ConTokens(t *testing.T) {
	env := newDefaultEnv()
	active, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return active, nil }
	env.userRepo.findProviderFn = func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
		return provider, nil
	}

	var capturedKey string
	env.tokenRepo.incrementFn = func(ctx context.Context, key string, window time.Duration) (int, error) {
		capturedKey = key
		return 1, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got application.LoginPasswordResponse
	decodeJSON(t, rec.Body, &got)
	if got.AccessToken != "access-token" || got.RefreshToken != "refresh-token" {
		t.Errorf("body: got %+v, want tokens del fake", got)
	}

	// extractClientIP debe tomar solo la primera IP de la cadena X-Forwarded-For
	wantKey := "login_limit:9.9.9.9:user@test.com"
	if capturedKey != wantKey {
		t.Errorf("rate limit key: got %q, want %q (extractClientIP debería usar la primera IP del header)", capturedKey, wantKey)
	}
}

func TestHandleLoginPassword_SinForwardedFor_UsaRemoteAddr(t *testing.T) {
	env := newDefaultEnv()
	active, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return active, nil }
	env.userRepo.findProviderFn = func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
		return provider, nil
	}
	var capturedKey string
	env.tokenRepo.incrementFn = func(ctx context.Context, key string, window time.Duration) (int, error) {
		capturedKey = key
		return 1, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"email": "user@test.com", "password": "Password1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
	req.RemoteAddr = "203.0.113.5:54321"
	rec := httptest.NewRecorder()

	h.HandleLoginPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	wantKey := "login_limit:203.0.113.5:54321:user@test.com"
	if capturedKey != wantKey {
		t.Errorf("rate limit key: got %q, want %q (sin X-Forwarded-For debería caer a RemoteAddr)", capturedKey, wantKey)
	}
}

// ─── HandleVerifyEmail ──────────────────────────────────────────────────────

func TestHandleVerifyEmail_SinTokenEnQuery_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	rec := httptest.NewRecorder()

	h.HandleVerifyEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleVerifyEmail_TokenNoEncontrado_Retorna404(t *testing.T) {
	// findVerificationTokenFn no configurada -> devuelve nil,nil -> ErrTokenNotFound
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token=no-existe", nil)
	rec := httptest.NewRecorder()

	h.HandleVerifyEmail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleVerifyEmail_TokenExpirado_Retorna400(t *testing.T) {
	env := newDefaultEnv()
	expired := domain.ReconstructVerificationToken("tok-1", domain.TypeEmailVerification, "user-1", time.Now().Add(-1*time.Hour), nil)
	env.userRepo.findVerificationTokenFn = func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
		return expired, nil
	}
	h := newTestHandler(env)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token=tok-1", nil)
	rec := httptest.NewRecorder()

	h.HandleVerifyEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleVerifyEmail_HappyPath_Retorna200(t *testing.T) {
	env := newDefaultEnv()
	valid := domain.ReconstructVerificationToken("tok-1", domain.TypeEmailVerification, "user-1", time.Now().Add(24*time.Hour), nil)
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	env.userRepo.findVerificationTokenFn = func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
		return valid, nil
	}
	env.userRepo.findByIDFn = func(ctx context.Context, id string) (*domain.User, error) { return pending, nil }
	h := newTestHandler(env)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token=tok-1", nil)
	rec := httptest.NewRecorder()

	h.HandleVerifyEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got application.LoginPasswordResponse
	decodeJSON(t, rec.Body, &got)
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Errorf("body: tokens vacíos, got %+v", got)
	}
}

// ─── HandleGoogleLogin ──────────────────────────────────────────────────────

func TestHandleGoogleLogin_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/google", bytes.NewBufferString("{invalido"))
	rec := httptest.NewRecorder()

	h.HandleGoogleLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGoogleLogin_LinkVerificationRequired_Retorna409(t *testing.T) {
	env := newDefaultEnv()
	pending, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	env.userRepo.findByEmailFn = func(ctx context.Context, email string) (*domain.User, error) { return pending, nil }
	env.identitySvc.verifyGoogleFn = func(ctx context.Context, code string) (*ports.SSOResult, error) {
		return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
	}
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"code": "auth-code"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/google", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleGoogleLogin(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleGoogleLogin_ErrorGenerico_Retorna422(t *testing.T) {
	env := newDefaultEnv()
	boom := errors.New("falló la comunicación con Google")
	env.identitySvc.verifyGoogleFn = func(ctx context.Context, code string) (*ports.SSOResult, error) { return nil, boom }
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"code": "bad-code"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/google", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleGoogleLogin(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleGoogleLogin_HappyPath_Retorna200(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"code": "good-code"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/google", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleGoogleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got application.LoginPasswordResponse
	decodeJSON(t, rec.Body, &got)
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Errorf("body: tokens vacíos, got %+v", got)
	}
}

// ─── HandleMetaLogin ────────────────────────────────────────────────────────

func TestHandleMetaLogin_JSONInvalido_Retorna400(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/meta", bytes.NewBufferString("{invalido"))
	rec := httptest.NewRecorder()

	h.HandleMetaLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleMetaLogin_ErrorGenerico_Retorna422(t *testing.T) {
	env := newDefaultEnv()
	boom := errors.New("token de Meta inválido")
	env.identitySvc.verifyMetaFn = func(ctx context.Context, accessToken string) (*ports.SSOResult, error) { return nil, boom }
	h := newTestHandler(env)

	body, _ := json.Marshal(map[string]string{"access_token": "bad-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/meta", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleMetaLogin(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleMetaLogin_HappyPath_Retorna200(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	body, _ := json.Marshal(map[string]string{"access_token": "good-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sso/meta", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	h.HandleMetaLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got application.LoginPasswordResponse
	decodeJSON(t, rec.Body, &got)
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Errorf("body: tokens vacíos, got %+v", got)
	}
}

// ─── HandleSSOConfig ────────────────────────────────────────────────────────

func TestHandleSSOConfig_Retorna200ConConfiguracion(t *testing.T) {
	h := newTestHandler(newDefaultEnv())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sso/config", nil)
	rec := httptest.NewRecorder()

	h.HandleSSOConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var got httpapi.SSOPublicConfig
	decodeJSON(t, rec.Body, &got)
	if got.GoogleClientID != "google-client-id" || got.MetaAppID != "meta-app-id" {
		t.Errorf("body: got %+v, want la config armada en newTestHandler", got)
	}
}
