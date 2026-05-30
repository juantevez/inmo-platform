package ports

import (
	"context"
)

// SSOResult envuelve la información estandarizada que recuperamos de cualquier proveedor OAuth
type SSOResult struct {
	ProviderUserID string // El 'sub' en Google o el 'id' en Meta
	Email          string // Email del usuario (¡Ojo! Puede venir vacío en Meta)
	Name           string // Nombre completo del usuario
	AvatarURL      string // Foto de perfil si está disponible
}

type IdentityService interface {
	// VerifyGoogleCode intercambia el código de autorización por el perfil del usuario en Google (UC-04)
	VerifyGoogleCode(ctx context.Context, code string) (*SSOResult, error)

	// VerifyMetaToken valida el token de acceso contra la Graph API de Meta (UC-05)
	VerifyMetaToken(ctx context.Context, accessToken string) (*SSOResult, error)
}
