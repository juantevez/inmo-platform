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
	ErrTokenNotFound = errors.New("el token de verificación no existe")
)

// VerifyEmailCommand transporta el token crudo que vino por parámetro en la URL
type VerifyEmailCommand struct {
	TokenValue string
}

type VerifyEmailUseCase struct {
	userRepo       ports.UserRepository
	tokenRepo      ports.TokenRepository
	tokenService   TokenService
	eventPublisher ports.EventPublisher
	uuidGen        UUIDGenerator
}

func NewVerifyEmailUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	tokenService TokenService,
	publisher ports.EventPublisher,
	uuidGen UUIDGenerator,
) *VerifyEmailUseCase {
	return &VerifyEmailUseCase{
		userRepo:       userRepo,
		tokenRepo:      tokenRepo,
		tokenService:   tokenService,
		eventPublisher: publisher,
		uuidGen:        uuidGen,
	}
}

func (uc *VerifyEmailUseCase) Execute(ctx context.Context, cmd VerifyEmailCommand) (*LoginPasswordResponse, error) {
	// 1. Buscar el token en la base de datos (Filtramos por tipo EMAIL_VERIFICATION)
	token, err := uc.userRepo.FindVerificationToken(ctx, cmd.TokenValue, domain.TypeEmailVerification)
	if err != nil {
		return nil, fmt.Errorf("error al buscar el token de verificación: %w", err)
	}
	if token == nil {
		return nil, ErrTokenNotFound // Retornará 404 o 410 según decidas en el handler HTTP
	}

	// 2. Validar reglas de negocio del token (Expiración y re-uso) usando el Dominio
	if err := token.Validate(); err != nil {
		if errors.Is(err, domain.ErrTokenExpired) {
			return nil, errors.New("el link de verificación ha expirado (TTL 24h). Solicite uno nuevo") // 410 Gone
		}
		return nil, err
	}

	// 3. Buscar el Usuario dueño de ese token
	user, err := uc.userRepo.FindByID(ctx, token.UserID())
	if err != nil {
		return nil, fmt.Errorf("error al recuperar el usuario del token: %w", err)
	}
	if user == nil {
		return nil, errors.New("el usuario asociado al token ya no existe")
	}

	// 4. Modificar el estado del Agregado User a ACTIVE (El dominio valida que no esté SUSPENDED)
	if err := user.Activate(); err != nil {
		return nil, err
	}

	// 5. Marcar el token como consumido en memoria
	_ = token.Use()

	// 6. Persistencia atómica de la activación y la quema del token
	// Idealmente tu UserRepository manejará esto dentro de un mismo bloque transaccional de Postgres
	err = uc.userRepo.Update(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("error al actualizar el estado del usuario: %w", err)
	}

	err = uc.userRepo.UpdateVerificationToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("error al actualizar el token de verificación: %w", err)
	}

	// Definimos el rol por defecto para los registros automáticos por SSO
	initialRoles := []string{"INQUILINO"}
	// 7. Emitir los tokens de sesión directa para comodidad del usuario (UX)
	accessToken, err := uc.tokenService.GenerateAccessToken(user.ID(), initialRoles)
	if err != nil {
		return nil, fmt.Errorf("error al generar access token post-verificación: %w", err)
	}

	refreshToken, err := uc.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("error al generar refresh token post-verificación: %w", err)
	}

	// 8. Guardar sesión en Redis (7 días de TTL)
	err = uc.tokenRepo.SetRefreshToken(ctx, refreshToken, user.ID(), 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("error al guardar sesión en caché: %w", err)
	}

	// 9. Publicar evento en NATS indicando que el mail fue validado con éxito
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.email.verified",
		UserID:    user.ID(),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"email": user.Email(),
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	// 10. Devolvemos el par de tokens usando la misma estructura de respuesta que el Login
	return &LoginPasswordResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
