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
	ssoResult, err := uc.identityService.VerifyMetaToken(ctx, cmd.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("falló la validación del token con Meta Graph API: %w", err)
	}

	if ssoResult.Email == "" {
		return nil, ErrMetaEmailMissing
	}

	user, err := uc.userRepo.FindByEmail(ctx, ssoResult.Email)
	if err != nil {
		return nil, fmt.Errorf("error al buscar usuario por email: %w", err)
	}

	// --- ESCENARIO A: usuario nuevo vía Meta (se registra como INTERESADO por defecto) ---
	if user == nil {
		userID := uc.uuidGen()
		providerID := uc.uuidGen()

		user, err = domain.NewUserFromSSO(userID, ssoResult.Email)
		if err != nil {
			return nil, err
		}

		metaProvider, err := domain.NewSSOProvider(providerID, userID, domain.ProviderMeta, ssoResult.ProviderUserID)
		if err != nil {
			return nil, err
		}

		_ = user.LinkProvider(metaProvider)

		// SSO auto-registra como INTERESADO. El usuario puede cambiar su rol
		// completando su perfil después del primer login.
		initialRoles := []string{"INTERESADO"}
		err = uc.userRepo.Save(ctx, user, metaProvider, initialRoles)
		if err != nil {
			return nil, fmt.Errorf("error al registrar usuario de Meta en BD: %w", err)
		}

		return uc.issueMetaTokens(ctx, user.ID(), "META_SIGNUP", cmd)
	}

	// --- EL USUARIO SÍ EXISTE ---
	metaProvider, err := uc.userRepo.FindProvider(ctx, user.ID(), domain.ProviderMeta)
	if err != nil {
		return nil, err
	}

	// --- ESCENARIO B: login recurrente ---
	if metaProvider != nil {
		if metaProvider.ProviderUserID() != ssoResult.ProviderUserID {
			return nil, errors.New("el ID de cuenta de Meta no coincide con el registrado")
		}
		if user.Status() == domain.StatusSuspended {
			return nil, errors.New("cuenta suspendida por administración")
		}
		return uc.issueMetaTokens(ctx, user.ID(), "META_LOGIN", cmd)
	}

	// --- ESCENARIO C: account linking ---
	if user.Status() == domain.StatusPendingVerification {
		return nil, errors.New("el usuario local no está verificado. Verifique su correo antes de vincular con Meta")
	}
	if user.Status() == domain.StatusSuspended {
		return nil, errors.New("cuenta suspendida por administración")
	}

	providerID := uc.uuidGen()
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

// issueMetaTokens FIX: ahora consulta los roles reales del usuario desde la DB
// en lugar de hardcodear nil o INQUILINO.
func (uc *LoginSSOMetaUseCase) issueMetaTokens(ctx context.Context, userID string, mode string, cmd LoginSSOMetaCommand) (*LoginPasswordResponse, error) {
	// FIX: buscar los roles reales del usuario para estamparlos en el JWT
	roles, err := uc.userRepo.FindRolesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("error al recuperar roles del usuario: %w", err)
	}

	accessToken, err := uc.tokenService.GenerateAccessToken(userID, roles)
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

	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.logged_in",
		UserID:    userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"ip":         cmd.ClientIP,
			"user_agent": cmd.UserAgent,
			"provider":   "META",
			"sso_mode":   mode,
			"roles":      roles,
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	return &LoginPasswordResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
