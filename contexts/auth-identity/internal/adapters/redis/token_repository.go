package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNil = errors.New("key no encontrada en cache")

type RedisTokenRepository struct {
	client *redis.Client
}

// NewRedisTokenRepository inicializa el adaptador con el cliente oficial de go-redis
func NewRedisTokenRepository(client *redis.Client) *RedisTokenRepository {
	return &RedisTokenRepository{client: client}
}

// SetRefreshToken guarda el refresh token vinculando el tokenID (key) al userID (value) (UC-03/05/09)
func (r *RedisTokenRepository) SetRefreshToken(ctx context.Context, tokenID string, userID string, ttl time.Duration) error {
	key := fmt.Sprintf("refresh_token:%s", tokenID)

	err := r.client.Set(ctx, key, userID, ttl).Err()
	if err != nil {
		return fmt.Errorf("error al guardar refresh token en Redis: %w", err)
	}
	return nil
}

// GetRefreshToken recupera el userID dueño de la sesión actual (UC-09)
func (r *RedisTokenRepository) GetRefreshToken(ctx context.Context, tokenID string) (string, error) {
	key := fmt.Sprintf("refresh_token:%s", tokenID)

	userID, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", fmt.Errorf("token inexistente o expirado: %w", err)
	}
	if err != nil {
		return "", err
	}
	return userID, nil
}

// DeleteRefreshToken revoca un refresh token específico (UC-10 / UC-09 RTR)
func (r *RedisTokenRepository) DeleteRefreshToken(ctx context.Context, tokenID string) error {
	key := fmt.Sprintf("refresh_token:%s", tokenID)
	return r.client.Del(ctx, key).Err()
}

// DeleteAllRefreshTokens invalida todas las sesiones activas del usuario de forma segura (Logout Global)
func (r *RedisTokenRepository) DeleteAllRefreshTokens(ctx context.Context, userID string) error {
	var cursor uint64
	var keys []string
	var err error

	// Usamos un ciclo infinito para barrer las llaves usando SCAN en batches pequeños (ej: de a 100)
	// Esto evita bloquear el hilo único de procesamiento de Redis.
	for {
		// Buscamos llaves que matcheen con el prefijo de nuestros refresh tokens
		keys, cursor, err = r.client.Scan(ctx, cursor, "refresh_token:*", 100).Result()
		if err != nil {
			return fmt.Errorf("error al escanear sesiones en Redis: %w", err)
		}

		// Por cada llave encontrada, verificamos si el valor (el UserID) coincide con el solicitado
		for _, key := range keys {
			storedUserID, err := r.client.Get(ctx, key).Result()
			if err == nil && storedUserID == userID {
				// Si coincide, lo borramos inmediatamente (Logout del dispositivo)
				_ = r.client.Del(ctx, key)
			}
		}

		// Si el cursor vuelve a ser 0, significa que Redis terminó de barrer toda la base de datos
		if cursor == 0 {
			break
		}
	}

	return nil
}

// AddToBlocklist mete el JWT revocado en la lista negra por el tiempo restante que le quede de vida (UC-10)
func (r *RedisTokenRepository) AddToBlocklist(ctx context.Context, tokenStr string, ttl time.Duration) error {
	key := fmt.Sprintf("blocklist:%s", tokenStr)
	return r.client.Set(ctx, key, "revoked", ttl).Err()
}

// IsInBlocklist chequea si el token actual fue invalidado vía logout (Para el middleware de Auth)
func (r *RedisTokenRepository) IsInBlocklist(ctx context.Context, tokenStr string) (bool, error) {
	key := fmt.Sprintf("blocklist:%s", tokenStr)

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// IncrementLoginAttempts maneja tu rate-limiting de intentos fallidos de forma atómica y distribuida (UC-03)
func (r *RedisTokenRepository) IncrementLoginAttempts(ctx context.Context, key string, window time.Duration) (int, error) {
	pipe := r.client.Pipeline()

	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("falló la ejecución atómica del rate limit: %w", err)
	}

	return int(incr.Val()), nil
}

// ClearLoginAttempts elimina el contador de bloqueos cuando el login es exitoso (UC-03)
func (r *RedisTokenRepository) ClearLoginAttempts(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
