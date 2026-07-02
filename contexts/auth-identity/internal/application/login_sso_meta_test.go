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

// ─── Helpers ────────────────────────────────────────────────────────────────

func newMetaUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	identitySvc ports.IdentityService,
	tokenSvc application.TokenService,
	publisher ports.EventPublisher,
	uuidGen application.UUIDGenerator,
) *application.LoginSSOMetaUseCase {
	return application.NewLoginSSOMetaUseCase(userRepo, tokenRepo, identitySvc, tokenSvc, publisher, uuidGen)
}

// ─── Validaciones previas: token de Meta y email ──────────────────────────────

func TestMetaExecute_ErrorVerificandoToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("token de Meta expirado")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) { return nil, boom },
	}
	uc := newMetaUseCase(&fakeUserRepo{}, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "bad-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestMetaExecute_EmailVacio_RetornaErrMetaEmailMissing(t *testing.T) {
	// Meta puede devolver el perfil sin email si el usuario no le dio ese permiso.
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "", ProviderUserID: "meta-id-1"}, nil
		},
	}
	uc := newMetaUseCase(&fakeUserRepo{}, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(err, application.ErrMetaEmailMissing) {
		t.Fatalf("Execute: got %v, want ErrMetaEmailMissing", err)
	}
}

func TestMetaExecute_ErrorBuscandoUsuarioPorEmail_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@fb.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, boom },
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

// ─── Escenario A: usuario nuevo vía Meta ──────────────────────────────────────

func TestMetaExecute_UsuarioNuevo_ErrorAlGuardar_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("violación de constraint única")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "nuevo@fb.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			return boom
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestMetaExecute_UsuarioNuevo_HappyPath_RegistraYEmiteTokens(t *testing.T) {
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "nuevo@fb.com", ProviderUserID: "meta-id-1"}, nil
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
		// issueMetaTokens consulta los roles reales ya persistidos (simulamos que Save los dejó en INTERESADO).
		findRolesFn: func(ctx context.Context, userID string) ([]string, error) {
			return []string{"INTERESADO"}, nil
		},
	}
	tokenRepo := &fakeTokenRepo{}
	publisher := &fakeEventPublisher{}
	uc := newMetaUseCase(userRepo, tokenRepo, identitySvc, &fakeTokenService{}, publisher, sequentialUUIDGen("user-1", "prov-1", "evt-1"))

	resp, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{
		AccessToken: "good-token", ClientIP: "2.2.2.2", UserAgent: "safari",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.AccessToken != "access-token" || resp.RefreshToken != "refresh-token" {
		t.Errorf("Response: got %+v, want tokens por defecto del fake", resp)
	}

	if savedUser == nil {
		t.Fatal("Save: no fue invocado")
	}
	if savedUser.ID() != "user-1" || savedUser.Status() != domain.StatusActive || savedUser.Email() != "nuevo@fb.com" {
		t.Errorf("Save user: got ID=%q Status=%q Email=%q", savedUser.ID(), savedUser.Status(), savedUser.Email())
	}
	if savedProvider == nil || savedProvider.ID() != "prov-1" || savedProvider.Name() != domain.ProviderMeta || savedProvider.ProviderUserID() != "meta-id-1" {
		t.Errorf("Save provider: got %+v, want Meta/prov-1/meta-id-1", savedProvider)
	}
	// El registro automático vía Meta cae en INTERESADO (a diferencia de Google, que usa INQUILINO)
	if len(savedRoles) != 1 || savedRoles[0] != "INTERESADO" {
		t.Errorf("Save roles: got %v, want [INTERESADO]", savedRoles)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.Payload["sso_mode"] != "META_SIGNUP" {
		t.Errorf("Payload[sso_mode]: got %v, want %q", evt.Payload["sso_mode"], "META_SIGNUP")
	}
	if evt.Payload["provider"] != "META" {
		t.Errorf("Payload[provider]: got %v, want %q", evt.Payload["provider"], "META")
	}
	roles, ok := evt.Payload["roles"].([]string)
	if !ok || len(roles) != 1 || roles[0] != "INTERESADO" {
		t.Errorf("Payload[roles]: got %v, want [INTERESADO]", evt.Payload["roles"])
	}

	if !tokenRepo.setRefreshCalled || tokenRepo.setRefreshUserID != "user-1" || tokenRepo.setRefreshTTL != 7*24*time.Hour {
		t.Errorf("SetRefreshToken: got called=%v userID=%q ttl=%v", tokenRepo.setRefreshCalled, tokenRepo.setRefreshUserID, tokenRepo.setRefreshTTL)
	}
}

// ─── Escenario B: usuario existente ya vinculado a Meta (login recurrente) ────

func TestMetaExecute_UsuarioExistente_ErrorBuscandoProvider_RetornaErrorSinEnvolver(t *testing.T) {
	user, _ := buildActiveUserWithProvider(t)
	boom := errors.New("fallo consultando providers")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, boom
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, err := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestMetaExecute_UsuarioExistente_ProviderUserIDNoCoincide_RetornaError(t *testing.T) {
	user, _ := buildActiveUserWithProvider(t)
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-original")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-DIFERENTE"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr == nil || !strings.Contains(execErr.Error(), "no coincide") {
		t.Fatalf("Execute: got %v, want error de ID de Meta no coincidente", execErr)
	}
}

func TestMetaExecute_UsuarioExistente_Suspendido_RetornaError(t *testing.T) {
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr == nil || !strings.Contains(execErr.Error(), "suspendida") {
		t.Fatalf("Execute: got %v, want error de cuenta suspendida", execErr)
	}
}

func TestMetaExecute_UsuarioExistente_HappyPath_LoginRecurrente(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
		findRolesFn: func(ctx context.Context, userID string) ([]string, error) {
			return []string{"PROPIETARIO"}, nil
		},
	}
	var capturedRoles []string
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) {
			capturedRoles = roles
			return "signed-access", nil
		},
	}
	publisher := &fakeEventPublisher{}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, tokenSvc, publisher, fixedUUIDGen("evt-1"))

	resp, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("Response: tokens vacíos, got %+v", resp)
	}
	// Los roles del JWT deben reflejar los ya persistidos, no un valor hardcodeado
	if len(capturedRoles) != 1 || capturedRoles[0] != "PROPIETARIO" {
		t.Errorf("GenerateAccessToken roles: got %v, want [PROPIETARIO]", capturedRoles)
	}
	if len(publisher.published) != 1 || publisher.published[0].Payload["sso_mode"] != "META_LOGIN" {
		t.Errorf("PublishEvent: sso_mode esperado META_LOGIN, got %+v", publisher.published)
	}
}

// ─── Escenario C: usuario existente por EMAIL/otro medio que vincula Meta ─────

func TestMetaExecute_AccountLinking_PendingVerification_RetornaError(t *testing.T) {
	user, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr == nil || !strings.Contains(execErr.Error(), "no está verificado") {
		t.Fatalf("Execute: got %v, want error de verificación pendiente", execErr)
	}
}

func TestMetaExecute_AccountLinking_Suspendido_RetornaError(t *testing.T) {
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusSuspended), "", nil, time.Now())
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr == nil || !strings.Contains(execErr.Error(), "suspendida") {
		t.Fatalf("Execute: got %v, want error de cuenta suspendida", execErr)
	}
}

func TestMetaExecute_AccountLinking_ProviderYaVinculadoEnAgregado_RetornaError(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	existingMeta, err := domain.NewSSOProvider("prov-m-old", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	if err := user.LinkProvider(existingMeta); err != nil {
		t.Fatalf("LinkProvider setup: %v", err)
	}

	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("prov-new"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, domain.ErrProviderAlreadyLinked) {
		t.Fatalf("Execute: got %v, want %v", execErr, domain.ErrProviderAlreadyLinked)
	}
}

func TestMetaExecute_AccountLinking_ErrorAlPersistirVinculacion_RetornaErrorEnvuelto(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	boom := errors.New("fallo de escritura en identity_providers")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil
		},
		addProviderFn: func(ctx context.Context, userID string, provider *domain.IdentityProvider) error { return boom },
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("prov-new"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestMetaExecute_AccountLinking_HappyPath_VinculaYEmiteTokens(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
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
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, publisher, sequentialUUIDGen("prov-new", "evt-1"))

	resp, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if execErr != nil {
		t.Fatalf("Execute: error inesperado: %v", execErr)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("Response: tokens vacíos, got %+v", resp)
	}
	if addedUserID != "user-1" {
		t.Errorf("AddProvider userID: got %q, want %q", addedUserID, "user-1")
	}
	if addedProvider == nil || addedProvider.Name() != domain.ProviderMeta || addedProvider.ProviderUserID() != "meta-id-1" {
		t.Errorf("AddProvider provider: got %+v, want Meta/meta-id-1", addedProvider)
	}
	found := false
	for _, p := range user.Providers() {
		if p.Name() == domain.ProviderMeta {
			found = true
		}
	}
	if !found {
		t.Error("user.Providers(): el provider Meta debería quedar vinculado en el agregado")
	}
	if len(publisher.published) != 1 || publisher.published[0].Payload["sso_mode"] != "META_ACCOUNT_LINK" {
		t.Errorf("PublishEvent: sso_mode esperado META_ACCOUNT_LINK, got %+v", publisher.published)
	}
}

// ─── issueMetaTokens: fallas compartidas por las tres escenarios ──────────────

func TestMetaExecute_IssueTokens_ErrorBuscandoRoles_RetornaErrorEnvuelto(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("fallo consultando roles")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
		findRolesFn: func(ctx context.Context, userID string) ([]string, error) { return nil, boom },
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestMetaExecute_IssueTokens_ErrorGenerandoAccessToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("clave de firma inválida")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) { return "", boom },
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}

func TestMetaExecute_IssueTokens_ErrorGenerandoRefreshToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("fallo generando refresh token")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
	}
	tokenSvc := &fakeTokenService{
		generateRefreshFn: func() (string, error) { return "", boom },
	}
	uc := newMetaUseCase(userRepo, &fakeTokenRepo{}, identitySvc, tokenSvc, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}

func TestMetaExecute_IssueTokens_ErrorPersistiendoRefreshToken(t *testing.T) {
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	metaProvider, err := domain.NewSSOProvider("prov-m", user.ID(), domain.ProviderMeta, "meta-id-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}
	boom := errors.New("redis no disponible")
	identitySvc := &fakeIdentityService{
		verifyMetaFn: func(ctx context.Context, accessToken string) (*ports.SSOResult, error) {
			return &ports.SSOResult{Email: "user@test.com", ProviderUserID: "meta-id-1"}, nil
		},
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return user, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return metaProvider, nil
		},
	}
	tokenRepo := &fakeTokenRepo{
		setRefreshFn: func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error { return boom },
	}
	uc := newMetaUseCase(userRepo, tokenRepo, identitySvc, &fakeTokenService{}, &fakeEventPublisher{}, fixedUUIDGen("evt-1"))

	_, execErr := uc.Execute(context.Background(), application.LoginSSOMetaCommand{AccessToken: "good-token"})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}
