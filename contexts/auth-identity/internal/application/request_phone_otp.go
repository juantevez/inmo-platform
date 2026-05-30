package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"inmo.platform/contexts/auth-identity/internal/domain"
	"inmo.platform/contexts/auth-identity/internal/ports"
)

var (
	ErrOTPMaxAttemptsExceeded = errors.New("ha superado el límite máximo de 3 solicitudes de código por hora")
	ErrInvalidChannel         = errors.New("el canal de envío especificado no es válido (use SMS o WHATSAPP)")
)

// RequestPhoneOTPCommand recibe los datos para disparar el código temporal
type RequestPhoneOTPCommand struct {
	UserID  string
	Channel string // "SMS" o "WHATSAPP"
}

type RequestPhoneOTPUseCase struct {
	userRepo       ports.UserRepository
	tokenRepo      ports.TokenRepository
	eventPublisher ports.EventPublisher
	uuidGen        UUIDGenerator
}

func NewRequestPhoneOTPUseCase(
	userRepo ports.UserRepository,
	tokenRepo ports.TokenRepository,
	publisher ports.EventPublisher,
	uuidGen UUIDGenerator,
) *RequestPhoneOTPUseCase {
	return &RequestPhoneOTPUseCase{
		userRepo:       userRepo,
		tokenRepo:      tokenRepo,
		eventPublisher: publisher,
		uuidGen:        uuidGen,
	}
}

func (uc *RequestPhoneOTPUseCase) Execute(ctx context.Context, cmd RequestPhoneOTPCommand) error {
	// 1. Validar el canal solicitado
	if cmd.Channel != "SMS" && cmd.Channel != "WHATSAPP" {
		return ErrInvalidChannel
	}

	// 2. Buscar al usuario para verificar que tenga un teléfono cargado
	user, err := uc.userRepo.FindByID(ctx, cmd.UserID)
	if err != nil || user == nil {
		return errors.New("usuario no encontrado")
	}
	if user.Phone() == "" {
		return errors.New("el usuario no posee un número de teléfono registrado en su cuenta")
	}

	// 3. 🚀 APLICAR RATE LIMITING EN REDIS (UC-07: Máximo 3 envíos por número por hora)
	rateLimitKey := fmt.Sprintf("otp_limit:%s", user.Phone())
	attempts, err := uc.tokenRepo.IncrementLoginAttempts(ctx, rateLimitKey, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("error al verificar límite de OTP: %w", err)
	}
	if attempts > 3 {
		return ErrOTPMaxAttemptsExceeded // Retorna 429 Too Many Requests
	}

	// 4. Generar un OTP criptográficamente seguro de 6 dígitos de forma nativa en Go
	otpValue, err := generateNumericOTP(6)
	if err != nil {
		return fmt.Errorf("error al generar código de seguridad: %w", err)
	}

	// 5. Instanciar la entidad VerificationToken desde el Dominio (Asigna TTL de 10 min interno)
	otpToken := domain.NewPhoneOTP(otpValue, user.ID())

	// 6. Persistir el token de verificación en Postgres
	err = uc.userRepo.SaveVerificationToken(ctx, otpToken)
	if err != nil {
		return fmt.Errorf("error al guardar código OTP en BD: %w", err)
	}

	// 7. 🔌 INTEGRACIÓN LIMPIA (UC-08): Despachar evento a NATS JetStream
	// Tu microservicio whatsapp-bot-service (o Twilio) va a reaccionar asincrónicamente
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.phone_otp.requested",
		UserID:    user.ID(),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"phone":   user.Phone(),
			"otp":     otpToken.Value(),
			"channel": cmd.Channel, // El bot leerá este flag para saber si va por Baileys o SMS
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	return nil
}

// generateNumericOTP genera un string numérico aleatorio del largo especificado de forma segura
func generateNumericOTP(length int) (string, error) {
	result := ""
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		result += num.String()
	}
	return result, nil
}
