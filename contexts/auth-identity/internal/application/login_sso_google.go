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
	ErrLinkVerificationRequired = errors.New("el usuario local no está verificado. Inicie sesión de forma tradicional y verifique su correo antes de vincular con Google")
)

// LoginSSOGoogleCommand recibe el "Authorization Code" que el frontend
// consiguió de las pantallas de consentimiento de Google
type LoginSSOGoogleCommand struct {
	Code      string
	ClientIP  string
	UserAgent string
}

type LoginSSOGoogleUseCase struct {
	userRepo        ports.UserRepository
	tokenRepo       ports.TokenRepository
	identityService ports.IdentityService
	tokenService    TokenService
	eventPublisher  ports.EventPublisher
	uuidGen         UUIDGenerator
}

func NewLoginSSOGoogleUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	identityService ports.IdentityService,
	tokenService TokenService,
	publisher ports.EventPublisher,
	uuidGen UUIDGenerator,
) *LoginSSOGoogleUseCase {
	return &LoginSSOGoogleUseCase{
		userRepo:        userRepo,
		tokenRepo:       tokenRepo,
		identityService: identityService,
		tokenService:    tokenService,
		eventPublisher:  publisher,
		uuidGen:         uuidGen,
	}
}

func (uc *LoginSSOGoogleUseCase) Execute(ctx context.Context, cmd LoginSSOGoogleCommand) (*LoginPasswordResponse, error) {
	// 1. Intercambiar el Code por los datos reales del perfil en la API de Google (UC-04 Paso 3 y 4)
	ssoResult, err := uc.identityService.VerifyGoogleCode(ctx, cmd.Code)
	if err != nil {
		return nil, fmt.Errorf("falló la autenticación con el servidor de Google: %w", err)
	}

	// 2. Intentar buscar si el usuario ya existe en nuestra base por su Email
	user, err := uc.userRepo.FindByEmail(ctx, ssoResult.Email)
	if err != nil {
		return nil, fmt.Errorf("error al buscar usuario por email: %w", err)
	}

	// --- ESCENARIO A: EL USUARIO NO EXISTE EN NUESTRA DB (Registro Automático vía SSO) ---
	if user == nil {
		userID := uc.uuidGen()
		providerID := uc.uuidGen()

		// Nace directo en ACTIVE porque Google ya es un tercero de confianza que validó el buzón
		user, err = domain.NewUserFromSSO(userID, ssoResult.Email)
		if err != nil {
			return nil, err
		}

		// ◄ CORREGIDO: Pasamos userID como segundo argumento obligatorio
		googleProvider, err := domain.NewSSOProvider(providerID, userID, domain.ProviderGoogle, ssoResult.ProviderUserID)
		if err != nil {
			return nil, err
		}

		// ◄ CORREGIDO: Quitamos el '*' porque LinkProvider ahora recibe el puntero directo
		_ = user.LinkProvider(googleProvider)

		// Guardamos de forma atómica en Postgres
		err = uc.userRepo.Save(ctx, user, googleProvider)
		if err != nil {
			return nil, fmt.Errorf("error al registrar usuario de Google en BD: %w", err)
		}

		return uc.issueTokens(ctx, user.ID(), "GOOGLE_SIGNUP", cmd)
	}

	// --- EL USUARIO SÍ EXISTE EN NUESTRA DB ---

	// 3. Chequear si ya tiene el método de login GOOGLE guardado
	googleProvider, err := uc.userRepo.FindProvider(ctx, user.ID(), domain.ProviderGoogle)
	if err != nil {
		return nil, err
	}

	// --- ESCENARIO B: LOGIN RECURRENTE (Ya estaba vinculado) ---
	if googleProvider != nil {
		// Seguridad básica: validar que el ID de Google coincida por las dudas
		if googleProvider.ProviderUserID() != ssoResult.ProviderUserID {
			return nil, errors.New("el ID de cuenta de Google no coincide con el registrado")
		}

		if user.Status() == domain.StatusSuspended {
			return nil, errors.New("cuenta suspendida por administración")
		}

		return uc.issueTokens(ctx, user.ID(), "GOOGLE_LOGIN", cmd)
	}

	// --- ESCENARIO C: ACCOUNT LINKING (Existe por EMAIL tradicional, viene a vincular Google) ---

	// 🚀 APLICAMOS TU SUGERENCIA DE SEGURIDAD:
	// Si la cuenta local está PENDING_VERIFICATION, bloqueamos la vinculación automática.
	if user.Status() == domain.StatusPendingVerification {
		return nil, ErrLinkVerificationRequired // Retornará un 403 o 409 Custom en HTTP
	}

	if user.Status() == domain.StatusSuspended {
		return nil, errors.New("cuenta suspendida por administración")
	}

	// El usuario local está ACTIVE, procedemos a vincular el nuevo provider de forma segura
	providerID := uc.uuidGen()

	// ◄ CORREGIDO: Pasamos el user.ID() existente como segundo argumento obligatorio
	newGoogleProvider, err := domain.NewSSOProvider(providerID, user.ID(), domain.ProviderGoogle, ssoResult.ProviderUserID)
	if err != nil {
		return nil, err
	}

	// Intentamos meterlo al agregado (el dominio valida consistencia interna)
	// ◄ CORREGIDO: Quitamos el '*' para mantener firmas de punteros limpias
	if err := user.LinkProvider(newGoogleProvider); err != nil {
		return nil, err
	}

	// Persistimos la nueva credencial vinculada en la tabla identity_providers
	err = uc.userRepo.AddProvider(ctx, user.ID(), newGoogleProvider)
	if err != nil {
		return nil, fmt.Errorf("error al guardar vinculación de cuenta: %w", err)
	}

	return uc.issueTokens(ctx, user.ID(), "GOOGLE_ACCOUNT_LINK", cmd)
}

// issueTokens es una función auxiliar interna para evitar duplicar el bloque de generación de JWT y eventos
func (uc *LoginSSOGoogleUseCase) issueTokens(ctx context.Context, userID string, mode string, cmd LoginSSOGoogleCommand) (*LoginPasswordResponse, error) {
	accessToken, err := uc.tokenService.GenerateAccessToken(userID)
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

	// Avisamos a NATS lo que pasó (Auditoría/Métricas/Notificaciones)
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.logged_in",
		UserID:    userID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"ip":         cmd.ClientIP,
			"user_agent": cmd.UserAgent,
			"provider":   "GOOGLE",
			"sso_mode":   mode, // "GOOGLE_SIGNUP", "GOOGLE_LOGIN", o "GOOGLE_ACCOUNT_LINK"
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	return &LoginPasswordResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
