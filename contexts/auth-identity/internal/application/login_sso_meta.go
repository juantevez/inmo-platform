package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

var (
	ErrMetaEmailMissing = errors.New("la cuenta de Meta no tiene un correo electrónico asociado. Proporcione uno para continuar")
)

// LoginSSOMetaCommand recibe el Access Token de corta duración que el front
// obtuvo directamente desde el SDK de Facebook/Meta login
type LoginSSOMetaCommand struct {
	AccessToken string
	ClientIP    string
	UserAgent   string
}

type LoginSSOMetaUseCase struct {
	userRepo        ports.UserRepository
	tokenRepo       ports.TokenRepository
	identityService ports.IdentityService
	tokenService    TokenService
	eventPublisher  ports.EventPublisher
	uuidGen         UUIDGenerator
}

func NewLoginSSOMetaUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	identityService ports.IdentityService,
	tokenService TokenService,
	publisher ports.EventPublisher,
	uuidGen UUIDGenerator,
) *LoginSSOMetaUseCase {
	return &LoginSSOMetaUseCase{
		userRepo:        userRepo,
		tokenRepo:       tokenRepo,
		identityService: identityService,
		tokenService:    tokenService,
		eventPublisher:  publisher,
		uuidGen:         uuidGen,
	}
}

func (uc *LoginSSOMetaUseCase) Execute(ctx context.Context, cmd LoginSSOMetaCommand) (*LoginPasswordResponse, error) {
	// 1. Validar el token contra la Graph API de Meta (UC-05)
	ssoResult, err := uc.identityService.VerifyMetaToken(ctx, cmd.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("falló la validación del token con Meta Graph API: %w", err)
	}

	// 🚀 MANEJO DEL CASO ESPECIAL (UC-05): Cuenta de Meta sin Email configurado
	if ssoResult.Email == "" {
		return nil, ErrMetaEmailMissing // Clave para que el handler HTTP pida el correo en un paso extra
	}

	// 2. Buscar si el usuario ya existe por el Email devuelto por Meta
	user, err := uc.userRepo.FindByEmail(ctx, ssoResult.Email)
	if err != nil {
		return nil, fmt.Errorf("error al buscar usuario por email: %w", err)
	}

	// --- ESCENARIO A: EL USUARIO NO EXISTE EN NUESTRA DB (Registro Automático vía Meta) ---
	if user == nil {
		userID := uc.uuidGen()
		providerID := uc.uuidGen()

		// Nace directo en ACTIVE (Meta ya validó su identidad de origen)
		user, err = domain.NewUserFromSSO(userID, ssoResult.Email)
		if err != nil {
			return nil, err
		}

		// Creamos el proveedor exclusivo de META con la ID única de Facebook/Instagram
		// ◄ CORREGIDO: Pasamos userID como segundo argumento obligatorio
		metaProvider, err := domain.NewSSOProvider(providerID, userID, domain.ProviderMeta, ssoResult.ProviderUserID)
		if err != nil {
			return nil, err
		}

		// ◄ CORREGIDO: Quitamos el '*' porque LinkProvider ahora recibe el puntero directo
		_ = user.LinkProvider(metaProvider)

		// Definimos el rol por defecto para los registros automáticos por SSO
		initialRoles := []string{"INQUILINO"}

		// Guardamos transaccionalmente en Postgres
		err = uc.userRepo.Save(ctx, user, metaProvider, initialRoles)
		if err != nil {
			return nil, fmt.Errorf("error al registrar usuario de Meta en BD: %w", err)
		}

		return uc.issueMetaTokens(ctx, user.ID(), "META_SIGNUP", cmd)
	}

	// --- EL USUARIO SÍ EXISTE EN NUESTRA DB ---

	// 3. Chequear si ya tiene el método de login META vinculado
	metaProvider, err := uc.userRepo.FindProvider(ctx, user.ID(), domain.ProviderMeta)
	if err != nil {
		return nil, err
	}

	// --- ESCENARIO B: LOGIN RECURRENTE (Ya estaba vinculado de antes) ---
	if metaProvider != nil {
		if metaProvider.ProviderUserID() != ssoResult.ProviderUserID {
			return nil, errors.New("el ID de cuenta de Meta no coincide con el registrado")
		}

		if user.Status() == domain.StatusSuspended {
			return nil, errors.New("cuenta suspendida por administración")
		}

		return uc.issueMetaTokens(ctx, user.ID(), "META_LOGIN", cmd)
	}

	// --- ESCENARIO C: ACCOUNT LINKING (Existe por EMAIL o GOOGLE, viene a agregar Meta) ---

	// Aplicamos la misma regla de seguridad estricta: Bloquear si no validó su canal local
	if user.Status() == domain.StatusPendingVerification {
		return nil, errors.New("el usuario local no está verificado. Inicie sesión de forma tradicional y verifique su correo antes de vincular con Meta")
	}

	if user.Status() == domain.StatusSuspended {
		return nil, errors.New("cuenta suspendida por administración")
	}

	// Cuenta en estado ACTIVE, vinculamos el nuevo provider
	providerID := uc.uuidGen()

	// ◄ CORREGIDO: Pasamos el user.ID() existente como segundo argumento obligatorio
	newMetaProvider, err := domain.NewSSOProvider(providerID, user.ID(), domain.ProviderMeta, ssoResult.ProviderUserID)
	if err != nil {
		return nil, err
	}

	if err := user.LinkProvider(newMetaProvider); err != nil {
		return nil, err
	}

	err = uc.userRepo.AddProvider(ctx, user.ID(), newMetaProvider)
	if err != nil {
		return nil, fmt.Errorf("error al guardar vinculación de cuenta Meta: %w", err)
	}

	return uc.issueMetaTokens(ctx, user.ID(), "META_ACCOUNT_LINK", cmd)
}

// issueMetaTokens encapsula la generación de la sesión de Meta y los logs en NATS
func (uc *LoginSSOMetaUseCase) issueMetaTokens(ctx context.Context, userID string, mode string, cmd LoginSSOMetaCommand) (*LoginPasswordResponse, error) {
	accessToken, err := uc.tokenService.GenerateAccessToken(userID, nil)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	err = uc.tokenRepo.SetRefreshToken(ctx, refreshToken, userID, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	// Despachar evento de auditoría a NATS
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.logged_in",
		UserID:    userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"ip":         cmd.ClientIP,
			"user_agent": cmd.UserAgent,
			"provider":   "META",
			"sso_mode":   mode, // "META_SIGNUP", "META_LOGIN", o "META_ACCOUNT_LINK"
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	return &LoginPasswordResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
