package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"inmo.platform/contexts/auth-identity/internal/application"
	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newRegisterUseCase(userRepo ports.UserRepository, publisher ports.EventPublisher, uuidGen application.UUIDGenerator) *application.RegisterUserUseCase {
	return application.NewRegisterUserUseCase(userRepo, publisher, uuidGen)
}

// ─── Validación de rol ─────────────────────────────────────────────────────

func TestRegisterExecute_RolVacio_RetornaErrInvalidRole(t *testing.T) {
	uc := newRegisterUseCase(&fakeUserRepo{}, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, err := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "",
	})

	if !errors.Is(err, application.ErrInvalidRole) {
		t.Fatalf("Execute: got %v, want ErrInvalidRole", err)
	}
}

func TestRegisterExecute_RolInvalido_RetornaErrInvalidRole(t *testing.T) {
	uc := newRegisterUseCase(&fakeUserRepo{}, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, err := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "SUPERADMIN",
	})

	if !errors.Is(err, application.ErrInvalidRole) {
		t.Fatalf("Execute: got %v, want ErrInvalidRole", err)
	}
}

// ─── Chequeo de email existente ────────────────────────────────────────────

func TestRegisterExecute_ErrorBuscandoEmail_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, boom },
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, err := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestRegisterExecute_EmailYaRegistradoConProviderEmail_RetornaErrEmailAlreadyExists(t *testing.T) {
	existingUser, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	existingProvider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return existingUser, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return existingProvider, nil
		},
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "user@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if !errors.Is(execErr, application.ErrEmailAlreadyExists) {
		t.Fatalf("Execute: got %v, want ErrEmailAlreadyExists", execErr)
	}
}

func TestRegisterExecute_EmailRegistradoViaSSO_RetornaErrorDeSugerenciaSSO(t *testing.T) {
	existingUser, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return existingUser, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, nil // no tiene provider EMAIL, solo SSO
		},
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "user@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if execErr == nil || !strings.Contains(execErr.Error(), "SSO") {
		t.Fatalf("Execute: got %v, want error sugiriendo login con SSO", execErr)
	}
	// No debe confundirse con el error de email duplicado tradicional
	if errors.Is(execErr, application.ErrEmailAlreadyExists) {
		t.Error("Execute: el error de SSO no debería ser ErrEmailAlreadyExists")
	}
}

func TestRegisterExecute_EmailExistente_ErrorBuscandoProvider_TratadoComoSSO(t *testing.T) {
	// FindProvider ignora su error (uc.userRepo.FindProvider(ctx, ..., ...) usa "_" para el error),
	// así que un error técnico ahí termina cayendo en el mismo mensaje de "registrado vía SSO".
	existingUser, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return existingUser, nil },
		findProviderFn: func(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error) {
			return nil, errors.New("fallo de conexión")
		},
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, fixedUUIDGen("id-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "user@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if execErr == nil || !strings.Contains(execErr.Error(), "SSO") {
		t.Fatalf("Execute: got %v, want error sugiriendo login con SSO", execErr)
	}
}

// ─── Construcción del agregado ─────────────────────────────────────────────

func TestRegisterExecute_EmailInvalido_RetornaErrorDeDominio(t *testing.T) {
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1", "token-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "email-sin-arroba", Password: "Password1", Role: "INQUILINO",
	})

	if !errors.Is(execErr, domain.ErrInvalidEmail) {
		t.Fatalf("Execute: got %v, want %v", execErr, domain.ErrInvalidEmail)
	}
}

func TestRegisterExecute_PasswordDebil_RetornaErrorDeDominio(t *testing.T) {
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1", "token-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "corta", Role: "INQUILINO",
	})

	if execErr == nil || !strings.Contains(execErr.Error(), "8 caracteres") {
		t.Fatalf("Execute: got %v, want error de contraseña débil", execErr)
	}
}

// ─── Persistencia ───────────────────────────────────────────────────────────

func TestRegisterExecute_ErrorAlGuardarUsuario_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("violación de constraint única")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			return boom
		},
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1", "token-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

func TestRegisterExecute_ErrorAlGuardarTokenDeVerificacion_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("fallo de escritura en verification_tokens")
	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			return nil
		},
		saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return boom },
	}
	uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1", "token-1"))

	_, execErr := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "INQUILINO",
	})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", execErr, boom)
	}
}

// ─── Happy path ─────────────────────────────────────────────────────────────

func TestRegisterExecute_HappyPath_RegistraUsuarioYEmiteEvento(t *testing.T) {
	var savedUser *domain.User
	var savedProvider *domain.IdentityProvider
	var savedRoles []string
	var savedToken *domain.VerificationToken

	userRepo := &fakeUserRepo{
		findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			if email != "nuevo@test.com" {
				t.Errorf("FindByEmail: got %q, want %q", email, "nuevo@test.com")
			}
			return nil, nil
		},
		saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
			savedUser, savedProvider, savedRoles = user, provider, roles
			return nil
		},
		saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error {
			savedToken = token
			return nil
		},
	}
	publisher := &fakeEventPublisher{}
	uc := newRegisterUseCase(userRepo, publisher, sequentialUUIDGen("user-1", "prov-1", "token-1", "evt-1"))

	resp, err := uc.Execute(context.Background(), application.RegisterUserCommand{
		Email: "nuevo@test.com", Password: "Password1", Role: "PROPIETARIO",
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.UserID != "user-1" || resp.Role != "PROPIETARIO" {
		t.Errorf("Response: got %+v, want UserID=user-1 Role=PROPIETARIO", resp)
	}

	// El agregado nace PENDING_VERIFICATION con el ID de la primera llamada al generador
	if savedUser == nil {
		t.Fatal("Save: no fue invocado")
	}
	if savedUser.ID() != "user-1" {
		t.Errorf("Save user.ID(): got %q, want %q", savedUser.ID(), "user-1")
	}
	if savedUser.Status() != domain.StatusPendingVerification {
		t.Errorf("Save user.Status(): got %q, want %q", savedUser.Status(), domain.StatusPendingVerification)
	}

	// El provider EMAIL persistido usa el segundo UUID y valida la contraseña ingresada
	if savedProvider == nil {
		t.Fatal("Save: provider no fue pasado")
	}
	if savedProvider.ID() != "prov-1" || savedProvider.Name() != domain.ProviderEmail {
		t.Errorf("Save provider: got ID=%q Name=%q, want ID=prov-1 Name=EMAIL", savedProvider.ID(), savedProvider.Name())
	}
	if !savedProvider.VerifyPassword("Password1") {
		t.Error("Save provider: el hash guardado no valida la contraseña original")
	}

	// El rol persistido es el elegido por el usuario, no un hardcodeo
	if len(savedRoles) != 1 || savedRoles[0] != "PROPIETARIO" {
		t.Errorf("Save roles: got %v, want [PROPIETARIO]", savedRoles)
	}

	// El token de verificación usa el tercer UUID y apunta al usuario recién creado
	if savedToken == nil {
		t.Fatal("SaveVerificationToken: no fue invocado")
	}
	if savedToken.Value() != "token-1" || savedToken.UserID() != "user-1" || savedToken.Type() != domain.TypeEmailVerification {
		t.Errorf("SaveVerificationToken: got Value=%q UserID=%q Type=%q", savedToken.Value(), savedToken.UserID(), savedToken.Type())
	}

	// El evento de auditoría lleva el email, el rol elegido y el token de verificación
	if len(publisher.published) != 1 {
		t.Fatalf("PublishEvent: got %d eventos, want 1", len(publisher.published))
	}
	evt := publisher.published[0]
	if evt.EventID != "evt-1" {
		t.Errorf("EventID: got %q, want %q", evt.EventID, "evt-1")
	}
	if evt.Name != "auth.user.created" {
		t.Errorf("Name: got %q, want %q", evt.Name, "auth.user.created")
	}
	if evt.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", evt.UserID, "user-1")
	}
	if evt.Payload["email"] != "nuevo@test.com" {
		t.Errorf("Payload[email]: got %v, want %q", evt.Payload["email"], "nuevo@test.com")
	}
	if evt.Payload["role"] != "PROPIETARIO" {
		t.Errorf("Payload[role]: got %v, want %q", evt.Payload["role"], "PROPIETARIO")
	}
	if evt.Payload["verification_token"] != "token-1" {
		t.Errorf("Payload[verification_token]: got %v, want %q", evt.Payload["verification_token"], "token-1")
	}
}

func TestRegisterExecute_HappyPath_TodosLosRolesValidosSonAceptados(t *testing.T) {
	roles := []string{"INQUILINO", "PROPIETARIO", "AGENTE", "PROVEEDOR", "INTERESADO"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			userRepo := &fakeUserRepo{
				findByEmailFn: func(ctx context.Context, email string) (*domain.User, error) { return nil, nil },
				saveFn: func(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error {
					return nil
				},
				saveVerificationTokenFn: func(ctx context.Context, token *domain.VerificationToken) error { return nil },
			}
			uc := newRegisterUseCase(userRepo, &fakeEventPublisher{}, sequentialUUIDGen("user-1", "prov-1", "token-1", "evt-1"))

			resp, err := uc.Execute(context.Background(), application.RegisterUserCommand{
				Email: "nuevo@test.com", Password: "Password1", Role: role,
			})

			if err != nil {
				t.Fatalf("Execute(%q): error inesperado: %v", role, err)
			}
			if resp.Role != role {
				t.Errorf("Response.Role: got %q, want %q", resp.Role, role)
			}
		})
	}
}
