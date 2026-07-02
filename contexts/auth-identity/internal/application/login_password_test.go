package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

// ─── Fakes de los puertos ──────────────────────────────────────────────────────
//
// Se usan fakes basados en funciones (en vez de una librería de mocks) para
// poder configurar por test solo el comportamiento relevante y detectar
// llamadas no esperadas devolviendo un error explícito.

type fakeUserRepo struct {
	findByIDFn                func(ctx context.Context, id string) (*domain.User, error)
	findByEmailFn             func(ctx context.Context, email string) (*domain.User, error)
	findProviderFn            func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error)
	findRolesFn               func(ctx context.Context, userID string) ([]string, error)
	saveFn                    func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error
	addProviderFn             func(ctx context.Context, userID string, provider *domain.IdentityProvider) error
	saveVerificationTokenFn   func(ctx context.Context, token *domain.VerificationToken) error
	updateFn                  func(ctx context.Context, user *domain.User) error
	findVerificationTokenFn   func(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error)
	updateVerificationTokenFn func(ctx context.Context, token *domain.VerificationToken) error
}

func (f *fakeUserRepo) Save(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
	if f.saveFn != nil {
		return f.saveFn(ctx, user, provider, roles)
	}
	return errors.New("Save: fixture no configurada")
}
func (f *fakeUserRepo) Update(ctx context.Context, user *domain.User) error {
	if f.updateFn != nil {
		return f.updateFn(ctx, user)
	}
	return errors.New("Update: fixture no configurada")
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, errors.New("FindByID: fixture no configurada")
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.findByEmailFn != nil {
		return f.findByEmailFn(ctx, email)
	}
	return nil, errors.New("FindByEmail: fixture no configurada")
}
func (f *fakeUserRepo) FindRolesByUserID(ctx context.Context, userID string) ([]string, error) {
	if f.findRolesFn != nil {
		return f.findRolesFn(ctx, userID)
	}
	return nil, nil
}
func (f *fakeUserRepo) FindProvider(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
	if f.findProviderFn != nil {
		return f.findProviderFn(ctx, userID, pType)
	}
	return nil, errors.New("FindProvider: fixture no configurada")
}
func (f *fakeUserRepo) FindByProviderKey(ctx context.Context, pType domain.ProviderType, providerUserID string) (*domain.User, error) {
	return nil, errors.New("FindByProviderKey: no debería invocarse durante el login")
}
func (f *fakeUserRepo) AddProvider(ctx context.Context, userID string, provider *domain.IdentityProvider) error {
	if f.addProviderFn != nil {
		return f.addProviderFn(ctx, userID, provider)
	}
	return errors.New("AddProvider: fixture no configurada")
}
func (f *fakeUserRepo) SaveVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	if f.saveVerificationTokenFn != nil {
		return f.saveVerificationTokenFn(ctx, token)
	}
	return errors.New("SaveVerificationToken: fixture no configurada")
}
func (f *fakeUserRepo) FindVerificationToken(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error) {
	if f.findVerificationTokenFn != nil {
		return f.findVerificationTokenFn(ctx, tokenValue, tType)
	}
	return nil, errors.New("FindVerificationToken: fixture no configurada")
}
func (f *fakeUserRepo) UpdateVerificationToken(ctx context.Context, token *domain.VerificationToken) error {
	if f.updateVerificationTokenFn != nil {
		return f.updateVerificationTokenFn(ctx, token)
	}
	return errors.New("UpdateVerificationToken: fixture no configurada")
}

type fakeTokenRepo struct {
	incrementFn     func(ctx context.Context, key string, window time.Duration) (int, error)
	clearFn         func(ctx context.Context, key string) error
	setRefreshFn    func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error
	getRefreshFn    func(ctx context.Context, tokenID string) (string, error)
	deleteRefreshFn func(ctx context.Context, tokenID string) error

	clearCalled         bool
	setRefreshCalled    bool
	setRefreshUserID    string
	setRefreshTTL       time.Duration
	deleteRefreshCalled bool
	deletedRefreshToken string
}

func (f *fakeTokenRepo) SetRefreshToken(ctx context.Context, tokenID string, userID string, ttl time.Duration) error {
	f.setRefreshCalled = true
	f.setRefreshUserID = userID
	f.setRefreshTTL = ttl
	if f.setRefreshFn != nil {
		return f.setRefreshFn(ctx, tokenID, userID, ttl)
	}
	return nil
}
func (f *fakeTokenRepo) GetRefreshToken(ctx context.Context, tokenID string) (string, error) {
	if f.getRefreshFn != nil {
		return f.getRefreshFn(ctx, tokenID)
	}
	return "", errors.New("GetRefreshToken: fixture no configurada")
}
func (f *fakeTokenRepo) DeleteRefreshToken(ctx context.Context, tokenID string) error {
	f.deleteRefreshCalled = true
	f.deletedRefreshToken = tokenID
	if f.deleteRefreshFn != nil {
		return f.deleteRefreshFn(ctx, tokenID)
	}
	return nil
}
func (f *fakeTokenRepo) DeleteAllRefreshTokens(ctx context.Context, userID string) error {
	return errors.New("DeleteAllRefreshTokens: no debería invocarse durante el login")
}
func (f *fakeTokenRepo) AddToBlocklist(ctx context.Context, tokenStr string, ttl time.Duration) error {
	return errors.New("AddToBlocklist: no debería invocarse durante el login")
}
func (f *fakeTokenRepo) IsInBlocklist(ctx context.Context, tokenStr string) (bool, error) {
	return false, errors.New("IsInBlocklist: no debería invocarse durante el login")
}
func (f *fakeTokenRepo) IncrementLoginAttempts(ctx context.Context, key string, window time.Duration) (int, error) {
	if f.incrementFn != nil {
		return f.incrementFn(ctx, key, window)
	}
	return 1, nil
}
func (f *fakeTokenRepo) ClearLoginAttempts(ctx context.Context, key string) error {
	f.clearCalled = true
	if f.clearFn != nil {
		return f.clearFn(ctx, key)
	}
	return nil
}

type fakeEventPublisher struct {
	published []ports.AuthEvent
	err       error
}

func (f *fakeEventPublisher) PublishEvent(ctx context.Context, event ports.AuthEvent) error {
	f.published = append(f.published, event)
	return f.err
}

type fakeTokenService struct {
	generateAccessFn  func(userID string, roles []string) (string, error)
	generateRefreshFn func() (string, error)
}

func (f *fakeTokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	if f.generateAccessFn != nil {
		return f.generateAccessFn(userID, roles)
	}
	return "access-token", nil
}
func (f *fakeTokenService) GenerateRefreshToken() (string, error) {
	if f.generateRefreshFn != nil {
		return f.generateRefreshFn()
	}
	return "refresh-token", nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

const testPassword = "Password1"

// buildActiveUserWithProvider arma un usuario ACTIVE junto con su provider EMAIL,
// listo para el camino feliz de login.
func buildActiveUserWithProvider(t *testing.T) (*domain.User, *domain.IdentityProvider) {
	t.Helper()
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", testPassword)
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	return user, provider
}

func newUseCase(userRepo ports.UserRepository, tokenRepo ports.TokenRepository, tokenSvc application.TokenService, publisher ports.EventPublisher, uuidGen application.UUIDGenerator) *application.LoginPasswordUseCase {
	return application.NewLoginPasswordUseCase(userRepo, tokenRepo, tokenSvc, publisher, uuidGen)
}

func fixedUUIDGen(id string) application.UUIDGenerator {
	return func() string { return id }
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestExecute_RateLimitExcedido_RetornaErrRateLimitExceeded(t *testing.T) {
	userRepo := &fakeUserRepo{}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) {
			return 6, nil // > 5 dispara el bloqueo
		},
	}
	publisher := &fakeEventPublisher{}
	uc := newUseCase(userRepo, tokenRepo, &fakeTokenService{}, publisher, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{
		Email: "user@test.com", Password: testPassword, ClientIP: "1.2.3.4",
	})

	if !errors.Is(err, application.ErrRateLimitExceeded) {
		t.Fatalf("Execute: got %v, want ErrRateLimitExceeded", err)
	}
	// No debería haber llegado a publicar ningún evento
	if len(publisher.published) != 0 {
		t.Errorf("PublishEvent: no debería llamarse cuando el rate limit corta el flujo")
	}
}

func TestExecute_ErrorEnRateLimit_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("redis caído")
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) {
			return 0, boom
		},
	}
	uc := newUseCase(&fakeUserRepo{}, tokenRepo, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestExecute_UsuarioNoExiste_RetornaErrInvalidCredentials(t *testing.T) {
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return nil, nil // usuario inexistente
		},
	}
	tokenRepo := &fakeTokenRepo{}
	uc := newUseCase(userRepo, tokenRepo, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "nadie@test.com", Password: testPassword})

	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("Execute: got %v, want ErrInvalidCredentials", err)
	}
}

func TestExecute_ErrorBuscandoUsuario_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return nil, boom
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestExecute_ProviderNoEncontrado_RetornaErrInvalidCredentials(t *testing.T) {
	user, _ := buildActiveUserWithProvider(t)
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil // el usuario nunca vinculó un provider EMAIL (ej: solo tiene Google)
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("Execute: got %v, want ErrInvalidCredentials", err)
	}
}

func TestExecute_ErrorBuscandoProvider_RetornaErrInvalidCredentials(t *testing.T) {
	// El caso de uso trata el error de FindProvider igual que "no encontrado":
	// devuelve el genérico de credenciales inválidas en vez de propagar el error técnico.
	user, _ := buildActiveUserWithProvider(t)
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, errors.New("boom")
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("Execute: got %v, want ErrInvalidCredentials", err)
	}
}

func TestExecute_PasswordIncorrecta_RetornaErrInvalidCredentials(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: "clave-incorrecta"})

	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("Execute: got %v, want ErrInvalidCredentials", err)
	}
}

func TestExecute_EmailNoVerificado_RetornaErrEmailNotVerified(t *testing.T) {
	// NewUser (a diferencia de NewUserFromSSO) nace en PENDING_VERIFICATION.
	user, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", testPassword)
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if !errors.Is(execErr, application.ErrEmailNotVerified) {
		t.Fatalf("Execute: got %v, want ErrEmailNotVerified", execErr)
	}
}

func TestExecute_UsuarioSuspendido_RetornaError(t *testing.T) {
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", testPassword)
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if execErr == nil || !strings.Contains(execErr.Error(), "suspendida") {
		t.Fatalf("Execute: got %v, want error de cuenta suspendida", execErr)
	}
	// No debe ser confundido con el error de credenciales inválidas
	if errors.Is(execErr, application.ErrInvalidCredentials) {
		t.Errorf("Execute: el error de suspensión no debería ser ErrInvalidCredentials")
	}
}

func TestExecute_ErrorBuscandoRoles_RetornaErrorEnvuelto(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	boom := errors.New("fallo consultando roles")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
		findRolesFn: func(ctx context.Context, userID string) ([]string, error) { return nil, boom },
	}
	tokenRepo := &fakeTokenRepo{}
	uc := newUseCase(userRepo, tokenRepo, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
	// Los intentos fallidos ya se limpiaron antes de ir a buscar roles
	if !tokenRepo.clearCalled {
		t.Errorf("ClearLoginAttempts: debería haberse llamado antes de buscar roles")
	}
}

func TestExecute_ErrorGenerandoAccessToken_RetornaErrorEnvuelto(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	boom := errors.New("clave de firma inválida")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) { return "", boom },
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestExecute_ErrorGenerandoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	boom := errors.New("fallo generando refresh token")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateRefreshFn: func() (string, error) { return "", boom },
	}
	uc := newUseCase(userRepo, &fakeTokenRepo{}, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestExecute_ErrorPersistiendoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	boom := errors.New("redis no disponible")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return provider, nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		setRefreshFn: func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error { return boom },
	}
	uc := newUseCase(userRepo, tokenRepo, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginPasswordCommand{Email: "user@test.com", Password: testPassword})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestExecute_HappyPath_RetornaTokensYEfectosSecundariosCorrectos(t *testing.T) {
	user, provider := buildActiveUserWithProvider(t)
	roles := []string{"PROPIETARIO", "INTERESADO"}

	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			if email != "user@test.com" {
				t.Errorf("FindByEmail: got %q, want %q", email, "user@test.com")
			}
			return user, nil
		},
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			if pType != domain.ProviderEmail {
				t.Errorf("FindProvider: got %q, want %q", pType, domain.ProviderEmail)
			}
			return provider, nil
		},
		findRolesFn: func(ctx context.Context, userID string) ([]string, error) {
			return roles, nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		incrementFn: func(ctx context.Context, key string, window time.Duration) (int, error) {
			wantKey := "login_limit:9.9.9.9:user@test.com"
			if key != wantKey {
				t.Errorf("IncrementLoginAttempts key: got %q, want %q", key, wantKey)
			}
			if window != 15*time.Minute {
				t.Errorf("IncrementLoginAttempts window: got %v, want %v", window, 15*time.Minute)
			}
			return 1, nil
		},
	}
	var capturedUserID string
	var capturedRoles []string
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) {
			capturedUserID = userID
			capturedRoles = roles
			return "signed-access-token", nil
		},
		generateRefreshFn: func() (string, error) { return "signed-refresh-token", nil },
	}
	publisher := &fakeEventPublisher{}

	uc := newUseCase(userRepo, tokenRepo, tokenSvc, publisher, fixedUUIDGen("evt-123"))

	resp, err := uc.Execute(context.Background(), application.LoginPasswordCommand{
		Email:     "user@test.com",
		Password:  testPassword,
		ClientIP:  "9.9.9.9",
		UserAgent: "curl/8.0",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.AccessToken != "signed-access-token" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "signed-access-token")
	}
	if resp.RefreshToken != "signed-refresh-token" {
		t.Errorf("RefreshToken: got %q, want %q", resp.RefreshToken, "signed-refresh-token")
	}

	// El access token debe firmarse con el ID del usuario y sus roles reales
	if capturedUserID != "user-1" {
		t.Errorf("GenerateAccessToken userID: got %q, want %q", capturedUserID, "user-1")
	}
	if len(capturedRoles) != 2 || capturedRoles[0] != "PROPIETARIO" || capturedRoles[1] != "INTERESADO" {
		t.Errorf("GenerateAccessToken roles: got %v, want %v", capturedRoles, roles)
	}

	// Efectos secundarios: se limpió el contador y se persistió el refresh token con TTL de 7 días
	if !tokenRepo.clearCalled {
		t.Error("ClearLoginAttempts: debería haberse llamado en el camino feliz")
	}
	if !tokenRepo.setRefreshCalled {
		t.Fatal("SetRefreshToken: debería haberse llamado en el camino feliz")
	}
	if tokenRepo.setRefreshUserID != "user-1" {
		t.Errorf("SetRefreshToken userID: got %q, want %q", tokenRepo.setRefreshUserID, "user-1")
	}
	if tokenRepo.setRefreshTTL != 7*24*time.Hour {
		t.Errorf("SetRefreshToken TTL: got %v, want %v", tokenRepo.setRefreshTTL, 7*24*time.Hour)
	}

	// Se publicó exactamente un evento de auditoría con el payload esperado
	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.EventID != "evt-123" {
		t.Errorf("EventID: got %q, want %q", evt.EventID, "evt-123")
	}
	if evt.Name != "auth.user.logged_in" {
		t.Errorf("Name: got %q, want %q", evt.Name, "auth.user.logged_in")
	}
	if evt.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", evt.UserID, "user-1")
	}
	if evt.Payload["ip"] != "9.9.9.9" {
		t.Errorf("Payload[ip]: got %v, want %q", evt.Payload["ip"], "9.9.9.9")
	}
	if evt.Payload["user_agent"] != "curl/8.0" {
		t.Errorf("Payload[user_agent]: got %v, want %q", evt.Payload["user_agent"], "curl/8.0")
	}
	if evt.Payload["provider"] != "EMAIL" {
		t.Errorf("Payload[provider]: got %v, want %q", evt.Payload["provider"], "EMAIL")
	}
}
