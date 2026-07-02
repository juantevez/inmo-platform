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

// ─── Fake de IdentityService ────────────────────────────────────────────────

type fakeIdentityService struct {
	verifyGoogleFn func(ctx context.Context, code string) (*ports.SSOResult, error)
	verifyMetaFn   func(ctx context.Context, accessToken string) (*ports.SSOResult, error)
}

func (f *fakeIdentityService) VerifyGoogleCode(ctx context.Context, code string) (*ports.SSOResult, error) {
	if f.verifyGoogleFn != nil {
		return f.verifyGoogleFn(ctx, code)
	}
	return nil, errors.New("VerifyGoogleCode: fixture no configurada")
}
func (f *fakeIdentityService) VerifyMetaToken(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
	if f.verifyMetaFn != nil {
		return f.verifyMetaFn(ctx, accessToken)
	}
	return nil, errors.New("VerifyMetaToken: fixture no configurada")
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newGoogleUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	identitySvc ports.IdentityService,
	tokenSvc application.TokenService,
	publisher ports.EventPublisher,
	uuidGen application.UUIDGenerator,
) *application.LoginSSOGoogleUseCase {
	return application.NewLoginSSOGoogleUseCase(userRepo, tokenRepo, identitySvc, tokenSvc, publisher, uuidGen)
}

// sequentialUUIDGen devuelve los IDs dados en orden en cada llamada sucesiva,
// simulando uuid.New() determinísticamente para las aserciones del test.
func sequentialUUIDGen(ids ...string) application.UUIDGenerator {
	i := 0
	return func() string {
		id := ids[i]
		if i < len(ids)-1 {
			i++
		}
		return id
	}
}

// ─── Escenario A: usuario nuevo (alta automática vía Google) ──────────────────

func TestGoogleExecute_ErrorVerificandoCodigo_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("code inválido o expirado")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) { return nil, boom },
	}
	uc := newGoogleUseCase(&fakeUserRepo{}, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "bad-code"})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestGoogleExecute_ErrorBuscandoUsuarioPorEmail_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@gmail.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, boom },
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestGoogleExecute_UsuarioNuevo_ErrorAlGuardar_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("violación de constraint única")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "nuevo@gmail.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			return boom
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestGoogleExecute_UsuarioNuevo_HappyPath_RegistraYEmiteTokens(t *testing.T) {
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "nuevo@gmail.com", ProviderUserID: "google-sub-1"}, nil
		},
	}

	var savedUser *domain.User
	var savedProvider *domain.IdentityProvider
	var savedRoles []string
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			savedUser, savedProvider, savedRoles = user, provider, roles
			return nil
		},
	}
	tokenRepo := &fakeTokenRepo{}
	publisher := &fakeEventPublisher{}
	// El caso de uso pide dos UUIDs (userID y providerID) antes de emitir el evento (que pide un tercero).
	uc := newGoogleUseCase(userRepo, tokenRepo, identitySvc, &fakeTokenService{}, publisher, sequentialUUIDGen("user-1", "prov-1", "evt-1"))

	resp, err := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{
		Code: "good-code", ClientIP: "1.1.1.1", UserAgent: "chrome",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.AccessToken != "access-token" || resp.RefreshToken != "refresh-token" {
		t.Errorf("Response: got %+v, want tokens por defecto del fake", resp)
	}

	// El nuevo usuario nace ACTIVE (confianza delegada a Google) con ID de la primera llamada al generador
	if savedUser == nil {
		t.Fatal("Save: no fue invocado")
	}
	if savedUser.ID() != "user-1" {
		t.Errorf("Save user.ID(): got %q, want %q", savedUser.ID(), "user-1")
	}
	if savedUser.Status() != domain.StatusActive {
		t.Errorf("Save user.Status(): got %q, want %q", savedUser.Status(), domain.StatusActive)
	}
	if savedUser.Email() != "nuevo@gmail.com" {
		t.Errorf("Save user.Email(): got %q, want %q", savedUser.Email(), "nuevo@gmail.com")
	}

	// El provider persistido debe ser GOOGLE con el ID de la segunda llamada al generador
	if savedProvider == nil {
		t.Fatal("Save: provider no fue pasado")
	}
	if savedProvider.ID() != "prov-1" {
		t.Errorf("Save provider.ID(): got %q, want %q", savedProvider.ID(), "prov-1")
	}
	if savedProvider.Name() != domain.ProviderGoogle {
		t.Errorf("Save provider.Name(): got %q, want %q", savedProvider.Name(), domain.ProviderGoogle)
	}
	if savedProvider.ProviderUserID() != "google-sub-1" {
		t.Errorf("Save provider.ProviderUserID(): got %q, want %q", savedProvider.ProviderUserID(), "google-sub-1")
	}

	// Rol por defecto para altas automáticas vía SSO
	if len(savedRoles) != 1 || savedRoles[0] != "INQUILINO" {
		t.Errorf("Save roles: got %v, want [INQUILINO]", savedRoles)
	}

	// El evento de auditoría debe llevar el modo GOOGLE_SIGNUP
	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.Payload["sso_mode"] != "GOOGLE_SIGNUP" {
		t.Errorf("Payload[sso_mode]: got %v, want %q", evt.Payload["sso_mode"], "GOOGLE_SIGNUP")
	}
	if evt.Payload["provider"] != "GOOGLE" {
		t.Errorf("Payload[provider]: got %v, want %q", evt.Payload["provider"], "GOOGLE")
	}
	if evt.UserID != "user-1" {
		t.Errorf("EventID.UserID: got %q, want %q", evt.UserID, "user-1")
	}

	// El SetRefreshToken debe haberse llamado con el ID del nuevo usuario y TTL de 7 días
	if !tokenRepo.setRefreshCalled || tokenRepo.setRefreshUserID != "user-1" || tokenRepo.setRefreshTTL != 7*24*time.Hour {
		t.Errorf("SetRefreshToken: got called=%v userID=%q ttl=%v", tokenRepo.setRefreshCalled, tokenRepo.setRefreshUserID, tokenRepo.setRefreshTTL)
	}
}

// ─── Escenario B: usuario existente ya vinculado a Google (login recurrente) ──

func TestGoogleExecute_UsuarioExistente_ErrorBuscandoProvider_RetornaErrorSinEnvolver(t *testing.T) {
	// A diferencia de otros pasos, este error se propaga tal cual (sin fmt.Errorf %w).
	user, _ := buildActiveUserWithProvider(t)
	boom := errors.New("fallo consultando providers")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, boom
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestGoogleExecute_UsuarioExistente_ProviderUserIDNoCoincide_RetornaError(t *testing.T) {
	user, _ := buildActiveUserWithProvider(t)
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-original")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			// Llega un 'sub' distinto al que está guardado — posible token robado/manipulado
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-DIFERENTE"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr == nil || !strings.Contains(execErr.Error(), "no coincide") {
		t.Fatalf("Execute: got %v, want error de ID de Google no coincidente", execErr)
	}
}

func TestGoogleExecute_UsuarioExistente_Suspendido_RetornaError(t *testing.T) {
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr == nil || !strings.Contains(execErr.Error(), "suspendida") {
		t.Fatalf("Execute: got %v, want error de cuenta suspendida", execErr)
	}
}

func TestGoogleExecute_UsuarioExistente_HappyPath_LoginRecurrente(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
		// Save/AddProvider no deberían tocarse en un login recurrente.
	}
	publisher := &fakeEventPublisher{}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, publisher, fixedUUIDGen("evt-1"))

	resp, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("Response: tokens vacíos, got %+v", resp)
	}
	if len(publisher.published) != 1 || publisher.published[0].Payload["sso_mode"] != "GOOGLE_LOGIN" {
		t.Errorf("PublishEvent: sso_mode esperado GOOGLE_LOGIN, got %+v", publisher.published)
	}
}

// ─── Escenario C: usuario existente por EMAIL que vincula Google ──────────────

func TestGoogleExecute_AccountLinking_PendingVerification_RetornaError(t *testing.T) {
	user, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil // todavía no tiene Google vinculado
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(execErr, application.ErrLinkVerificationRequired) {
		t.Fatalf("Execute: got %v, want ErrLinkVerificationRequired", execErr)
	}
}

func TestGoogleExecute_AccountLinking_Suspendido_RetornaError(t *testing.T) {
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr == nil || !strings.Contains(execErr.Error(), "suspendida") {
		t.Fatalf("Execute: got %v, want error de cuenta suspendida", execErr)
	}
}

func TestGoogleExecute_AccountLinking_ProviderYaVinculadoEnAgregado_RetornaError(t *testing.T) {
	// Caso de inconsistencia: el repo dice que no hay provider GOOGLE (por ej. una lectura
	// obsoleta), pero el agregado en memoria ya lo tiene enlazado. LinkProvider debe frenarlo.
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	existingGoogle, err := domain.NewSSOProvider("prov-g-old", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	if err := user.LinkProvider(existingGoogle); err != nil {
		t.Fatalf("LinkProvider setup: %v", err)
	}

	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("prov-new"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(execErr, domain.ErrProviderAlreadyLinked) {
		t.Fatalf("Execute: got %v, want %v", execErr, domain.ErrProviderAlreadyLinked)
	}
}

func TestGoogleExecute_AccountLinking_ErrorAlPersistirVinculacion_RetornaErrorEnvuelto(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	boom := errors.New("fallo de escritura en identity_providers")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
		addProviderFn: func(ctx context.Context, userID string, provider *domain.IdentityProvider) error { return boom },
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("prov-new"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr == nil || !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestGoogleExecute_AccountLinking_HappyPath_VinculaYEmiteTokens(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	var addedUserID string
	var addedProvider *domain.IdentityProvider
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
		addProviderFn: func(ctx context.Context, userID string, provider *domain.IdentityProvider) error {
			addedUserID, addedProvider = userID, provider
			return nil
		},
	}
	publisher := &fakeEventPublisher{}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, publisher, sequentialUUIDGen("prov-new", "evt-1"))

	resp, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("Response: tokens vacíos, got %+v", resp)
	}
	if addedUserID != "user-1" {
		t.Errorf("AddProvider userID: got %q, want %q", addedUserID, "user-1")
	}
	if addedProvider == nil || addedProvider.Name() != domain.ProviderGoogle || addedProvider.ProviderUserID() != "google-sub-1" {
		t.Errorf("AddProvider provider: got %+v, want Google/google-sub-1", addedProvider)
	}
	// El agregado en memoria también debe reflejar el nuevo provider vinculado
	found := false
	for _, p := range user.Providers() {
		if p.Name() == domain.ProviderGoogle {
			found = true
		}
	}
	if !found {
		t.Error("user.Providers(): el provider Google debería quedar vinculado en el agregado")
	}
	if len(publisher.published) != 1 || publisher.published[0].Payload["sso_mode"] != "GOOGLE_ACCOUNT_LINK" {
		t.Errorf("PublishEvent: sso_mode esperado GOOGLE_ACCOUNT_LINK, got %+v", publisher.published)
	}
}

// ─── issueTokens: fallas compartidas por las tres escenarios ──────────────────

func TestGoogleExecute_IssueTokens_ErrorGenerandoAccessToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("clave de firma inválida")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) { return "", boom },
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}

func TestGoogleExecute_IssueTokens_ErrorGenerandoRefreshToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("fallo generando refresh token")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateRefreshFn: func() (string, error) { return "", boom },
	}
	uc := newGoogleUseCase(userRepo, &fakeTokenRepo{}, identitySvc, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}

func TestGoogleExecute_IssueTokens_ErrorPersistiendoRefreshToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	googleProvider, err := domain.NewSSOProvider("prov-g", user.ID(), domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("redis no disponible")
	identitySvc := &fakeIdentityService{
		verifyGoogleFn: func(ctx context.Context, code string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "google-sub-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return googleProvider, nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		setRefreshFn: func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error { return boom },
	}
	uc := newGoogleUseCase(userRepo, tokenRepo, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOGoogleCommand{Code: "good-code"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}
