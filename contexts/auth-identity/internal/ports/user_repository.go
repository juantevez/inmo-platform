package ports

import (
	"context"

	"inmo.platform/contexts/auth-identity/internal/domain"
)

type UserRepository interface {
	// 🚀 ACTUALIZADO: Ahora recibe la lista de roles iniciales para la transacción atómica
	Save(ctx context.Context, user *domain.User, provider *domain.IdentityProvider, roles []string) error

	// Update actualiza el estado general del usuario (ej: pasar a ACTIVE, teléfono, etc) (UC-02/07)
	Update(ctx context.Context, user *domain.User) error

	// FindByID busca un usuario por su clave primaria
	FindByID(ctx context.Context, id string) (*domain.User, error)

	// FindByEmail busca un usuario núcleo por su correo electrónico (UC-01/04/05)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)

	// 🚀 NUEVO: Recupera la lista de strings con todos los roles asignados a un usuario (Usado en Login/JWT)
	FindRolesByUserID(ctx context.Context, userID string) ([]string, error)

	// FindProvider busca un método de login específico asociado a un usuario (UC-03)
	FindProvider(ctx context.Context, userID string, pType domain.ProviderType) (*domain.IdentityProvider, error)

	// FindByProviderKey busca si ya existe un ID externo registrado (ej: el 'sub' de Google) (UC-04/05)
	FindByProviderKey(ctx context.Context, pType domain.ProviderType, providerUserID string) (*domain.User, error)

	// AddProvider vincula un nuevo proveedor de identidad a un usuario existente (UC-06)
	AddProvider(ctx context.Context, userID string, provider *domain.IdentityProvider) error

	// SaveVerificationToken guarda el token UUID o OTP para posterior verificación (UC-01/07)
	SaveVerificationToken(ctx context.Context, token *domain.VerificationToken) error

	// FindVerificationToken busca un token para validar su existencia y estado (UC-02/07)
	FindVerificationToken(ctx context.Context, tokenValue string, tType domain.TokenType) (*domain.VerificationToken, error)

	// UpdateVerificationToken marca el token como usado (UC-02/07)
	UpdateVerificationToken(ctx context.Context, token *domain.VerificationToken) error
}
