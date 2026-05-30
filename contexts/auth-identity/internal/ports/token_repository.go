package ports

import (
	"context"
	"time"
)

type TokenRepository interface {
	// SetRefreshToken almacena el refresh token vinculado a un usuario con su TTL (ej: 7 días) (UC-03/05)
	SetRefreshToken(ctx context.Context, tokenID string, userID string, ttl time.Duration) error

	// GetRefreshToken recupera el userID asociado al refresh token. Si no existe, devuelve un error (UC-09)
	GetRefreshToken(ctx context.Context, tokenID string) (string, error)

	// DeleteRefreshToken elimina el refresh token de la sesión actual (UC-10)
	DeleteRefreshToken(ctx context.Context, tokenID string) error

	// DeleteAllRefreshTokens elimina todas las sesiones activas del usuario (Logout de todos los dispositivos) (UC-10 Variante)
	DeleteAllRefreshTokens(ctx context.Context, userID string) error

	// AddToBlocklist mete un Access Token (JWT) en la lista negra por el tiempo que le quede de vida (UC-10)
	AddToBlocklist(ctx context.Context, tokenStr string, ttl time.Duration) error

	// IsInBlocklist chequea si un token fue revocado vía logout (Para el Middleware HTTP de Auth)
	IsInBlocklist(ctx context.Context, tokenStr string) (bool, error)

	// IncrementLoginAttempts aumenta el contador de fallas de una combinación IP+Email. Retorna el conteo actual (UC-03)
	IncrementLoginAttempts(ctx context.Context, key string, window time.Duration) (int, error)

	// ClearLoginAttempts limpia el limitador cuando el usuario se loguea con éxito (UC-03)
	ClearLoginAttempts(ctx context.Context, key string) error
}
