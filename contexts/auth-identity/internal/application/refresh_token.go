package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/auth-identity/internal/ports"
)

var (
	ErrInvalidRefreshToken = errors.New("el token de actualización es inválido o ha expirado")
)

// RefreshTokenCommand recibe el token que el cliente tiene guardado (usualmente en una cookie HttpOnly)
type RefreshTokenCommand struct {
	RefreshToken string
}

type RefreshTokenUseCase struct {
	tokenRepo    ports.TokenRepository
	tokenService TokenService
}

func NewRefreshTokenUseCase(tokenRepo ports.TokenRepository, tokenService TokenService) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		tokenRepo:    tokenRepo,
		tokenService: tokenService,
	}
}

func (uc *RefreshTokenUseCase) Execute(ctx context.Context, cmd RefreshTokenCommand) (*LoginPasswordResponse, error) {
	if cmd.RefreshToken == "" {
		return nil, ErrInvalidRefreshToken
	}

	// 1. Ir a Redis a buscar el UserID asociado a ese Refresh Token (UC-09 Paso 2)
	userID, err := uc.tokenRepo.GetRefreshToken(ctx, cmd.RefreshToken)
	if err != nil {
		// Si Redis no lo encuentra o da error, el token no va más
		return nil, ErrInvalidRefreshToken // Retornará 401 Unauthorized en HTTP
	}

	// 2. 🚀 ROTACIÓN (RTR): Quemamos el token viejo inmediatamente antes de hacer cualquier otra cosa.
	// Si un atacante interceptó este token e intenta un ataque de repetición ("replay attack"),
	// se va a encontrar con que el token ya no existe en Redis.
	_ = uc.tokenRepo.DeleteRefreshToken(ctx, cmd.RefreshToken)

	// 3. Emitir el nuevo par de tokens (UC-09 Pasos 3 y 4)
	// Definimos el rol por defecto para los registros automáticos por SSO
	initialRoles := []string{"INQUILINO"}

	newAccessToken, err := uc.tokenService.GenerateAccessToken(userID, initialRoles)
	if err != nil {
		return nil, fmt.Errorf("error al rotar el access token: %w", err)
	}

	newRefreshToken, err := uc.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("error al rotar el refresh token: %w", err)
	}

	// 4. Registrar el NUEVO refresh token en Redis volviendo a setear los 7 días de TTL
	err = uc.tokenRepo.SetRefreshToken(ctx, newRefreshToken, userID, 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("error al persistir la nueva sesión en caché: %w", err)
	}

	// 5. Devolvemos el combo renovado al cliente
	return &LoginPasswordResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
	}, nil
}
