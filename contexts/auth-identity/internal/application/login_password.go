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
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrRateLimitExceeded  = errors.New("demasiados intentos fallidos. Cuenta bloqueada temporalmente por 15 minutos")
	ErrEmailNotVerified   = errors.New("debe verificar su correo electrónico antes de iniciar sesión")
)

// LoginPasswordCommand recibe los datos de entrada del controlador HTTP, incluyendo contexto de red
type LoginPasswordCommand struct {
	Email     string
	Password  string
	ClientIP  string
	UserAgent string
}

// LoginPasswordResponse devuelve el combo de tokens exigido por el requerimiento
type LoginPasswordResponse struct {
	AccessToken  string
	RefreshToken string
}

// TokenService define un puerto interno (o que puede ir a ports/) para la firma de JWTs
type TokenService interface {
	GenerateAccessToken(userID string) (string, error)
	GenerateRefreshToken() (string, error)
}

type LoginPasswordUseCase struct {
	userRepo       ports.UserRepository
	tokenRepo      ports.TokenRepository
	tokenService   TokenService
	eventPublisher ports.EventPublisher
	uuidGen        UUIDGenerator
}

func NewLoginPasswordUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	tokenService TokenService,
	publisher ports.EventPublisher,
	uuidGen UUIDGenerator,
) *LoginPasswordUseCase {
	return &LoginPasswordUseCase{
		userRepo:       userRepo,
		tokenRepo:      tokenRepo,
		tokenService:   tokenService,
		eventPublisher: publisher,
		uuidGen:        uuidGen,
	}
}

func (uc *LoginPasswordUseCase) Execute(ctx context.Context, cmd LoginPasswordCommand) (*LoginPasswordResponse, error) {
	// Generamos la clave única para el Rate Limit en Redis combinando IP + Email
	rateLimitKey := fmt.Sprintf("login_limit:%s:%s", cmd.ClientIP, cmd.Email)

	// 1. Validar el Rate Limiting (UC-03 Flujo Alternativo)
	// Si el contador en Redis ya superó los 5 intentos, cortamos directo sin ir a la base de datos
	// Pasamos un TTL de 15 minutos para la ventana de bloqueo
	attempts, err := uc.tokenRepo.IncrementLoginAttempts(ctx, rateLimitKey, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("error al verificar el limitador de accesos: %w", err)
	}
	if attempts > 5 {
		return nil, ErrRateLimitExceeded // Devuelve 429 Too Many Requests en HTTP
	}

	// 2. Buscar al usuario por Email
	user, err := uc.userRepo.FindByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("error en la búsqueda de usuario: %w", err)
	}

	// Mantenemos seguridad: Si el usuario no existe, tiramos el genérico para evitar enumeración de usuarios
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	// 3. Buscar el proveedor EMAIL de ese usuario para extraer el hash de Bcrypt
	provider, err := uc.userRepo.FindProvider(ctx, user.ID(), domain.ProviderEmail)
	if err != nil || provider == nil {
		return nil, ErrInvalidCredentials
	}

	// 4. Comparar hashes usando el método encapsulado del Dominio
	if !provider.VerifyPassword(cmd.Password) {
		return nil, ErrInvalidCredentials // Al retornar, el contador de Redis ya quedó incrementado
	}

	// 5. Validar el estado del ciclo de vida del Agregado User
	if user.Status() == domain.StatusPendingVerification {
		return nil, ErrEmailNotVerified // Retorna 403 Forbidden sugiriendo verificar email
	}
	if user.Status() == domain.StatusSuspended {
		return nil, errors.New("su cuenta ha sido suspendida por el administrador")
	}

	// 6. Éxito de autenticación: Limpiamos los intentos fallidos en Redis de forma inmediata
	_ = uc.tokenRepo.ClearLoginAttempts(ctx, rateLimitKey)

	// 7. Emitir los tokens (Access Token corto y Refresh Token largo)
	accessToken, err := uc.tokenService.GenerateAccessToken(user.ID())
	if err != nil {
		return nil, fmt.Errorf("error al generar access token: %w", err)
	}

	refreshToken, err := uc.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("error al generar refresh token: %w", err)
	}

	// 8. Guardar el Refresh Token en Redis con un TTL de 7 días
	err = uc.tokenRepo.SetRefreshToken(ctx, refreshToken, user.ID(), 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("error al persistir sesión en caché: %w", err)
	}

	// 9. Registrar e instrumentar el LoginEvent mandándolo asincrónicamente a NATS para auditoría
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.logged_in",
		UserID:    user.ID(),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"ip":         cmd.ClientIP,
			"user_agent": cmd.UserAgent,
			"provider":   "EMAIL",
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	// 10. Coronamos devolviendo el par de tokens
	return &LoginPasswordResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
