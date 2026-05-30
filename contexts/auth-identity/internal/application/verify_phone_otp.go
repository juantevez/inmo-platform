package application

import (
	"context"
	"errors"
	"fmt"

	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

var (
	ErrOTPNotFound = errors.New("el código de verificación ingresado es incorrecto")
)

// VerifyPhoneOTPCommand transporta los datos de entrada del formulario de verificación
type VerifyPhoneOTPCommand struct {
	UserID   string
	OTPValue string
}

type VerifyPhoneOTPUseCase struct {
	userRepo ports.UserRepository
}

func NewVerifyPhoneOTPUseCase(userRepo ports.UserRepository) *VerifyPhoneOTPUseCase {
	return &VerifyPhoneOTPUseCase{
		userRepo: userRepo,
	}
}

func (uc *VerifyPhoneOTPUseCase) Execute(ctx context.Context, cmd VerifyPhoneOTPCommand) error {
	if cmd.OTPValue == "" {
		return ErrOTPNotFound
	}

	// 1. Buscar el token en la base de datos filtrando por tipo PHONE_OTP
	token, err := uc.userRepo.FindVerificationToken(ctx, cmd.OTPValue, domain.TypePhoneOTP)
	if err != nil {
		return fmt.Errorf("error al validar código OTP: %w", err)
	}

	// Si el token no existe o pertenece a otro usuario, seguridad estricta: código inválido
	if token == nil || token.UserID() != cmd.UserID {
		return ErrOTPNotFound
	}

	// 2. Validar expiración y estado del token delegando al Dominio (Valida TTL de 10m y reuso)
	if err := token.Validate(); err != nil {
		if errors.Is(err, domain.ErrTokenExpired) {
			return errors.New("el código de seguridad ha expirado (TTL 10 minutos). Solicite uno nuevo")
		}
		return err
	}

	// 3. Buscar al usuario dueño de la sesión
	user, err := uc.userRepo.FindByID(ctx, token.UserID())
	if err != nil || user == nil {
		return errors.New("usuario no encontrado")
	}

	// 4. Quemar el OTP en memoria
	_ = token.Use()

	// 5. Actualizar el estado del usuario. Como no tenemos un método complejo,
	// usamos la técnica de mutación limpia en los adaptadores.
	// Para reflejarlo en el dominio, podríamos simularlo actualizando el puntero si fuera necesario.
	// En el adapter se mapeará el phone_verified_at = CURRENT_TIMESTAMP

	// 6. Impactar los cambios de forma transaccional en Postgres
	err = uc.userRepo.UpdateVerificationToken(ctx, token)
	if err != nil {
		return fmt.Errorf("error al procesar la baja del código de seguridad: %w", err)
	}

	// Forzamos a la base a marcar el teléfono como verificado pasando el usuario
	err = uc.userRepo.Update(ctx, user) // El repositorio se encargará de clavar el phone_verified_at real
	if err != nil {
		return fmt.Errorf("error al guardar la verificación telefónica en BD: %w", err)
	}

	return nil
}
