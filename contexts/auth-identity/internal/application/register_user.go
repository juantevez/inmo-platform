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
	ErrEmailAlreadyExists = errors.New("el correo electrónico ya se encuentra registrado")
)

// RegisterUserCommand transporta los datos de entrada desde el controlador HTTP
type RegisterUserCommand struct {
	Email    string
	Password string
}

// RegisterUserResponse retorna la data resultante que pide el requerimiento (sin JWT)
type RegisterUserResponse struct {
	UserID string
}

// UUIDGenerator define un pequeño puerto interno para desacoplar la creación de IDs/Tokens
type UUIDGenerator func() string

type RegisterUserUseCase struct {
	userRepo       ports.UserRepository
	eventPublisher ports.EventPublisher
	uuidGen        UUIDGenerator
}

func NewRegisterUserUseCase(repo ports.UserRepository, publisher ports.EventPublisher, uuidGen UUIDGenerator) *RegisterUserUseCase {
	return &RegisterUserUseCase{
		userRepo:       repo,
		eventPublisher: publisher,
		uuidGen:        uuidGen,
	}
}

func (uc *RegisterUserUseCase) Execute(ctx context.Context, cmd RegisterUserCommand) (*RegisterUserResponse, error) {
	// 1. Validar si el email ya existe en el sistema
	existingUser, err := uc.userRepo.FindByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("error al verificar existencia de email: %w", err)
	}

	// Flujo alternativo: El correo ya está ocupado
	if existingUser != nil {
		// Buscamos con qué proveedor está para poder darle una sugerencia exacta al cliente
		emailProvider, _ := uc.userRepo.FindProvider(ctx, existingUser.ID(), domain.ProviderEmail)
		if emailProvider != nil {
			return nil, ErrEmailAlreadyExists // Retorna 409 Conflict en la capa HTTP
		}

		return nil, errors.New("el email está registrado mediante SSO (Google/Meta). Intente iniciar sesión con ese proveedor")
	}

	// 2. Crear las IDs necesarias usando nuestro generador desacoplado
	userID := uc.uuidGen()
	providerID := uc.uuidGen()
	tokenValue := uc.uuidGen()

	// 3. Instanciar el Agregado User (Nace en estado PENDING_VERIFICATION)
	user, err := domain.NewUser(userID, cmd.Email)
	if err != nil {
		return nil, err // Devuelve errores de formato de mail, etc.
	}

	// 4. Instanciar el Provider genérico configurado como EMAIL
	// (Asegurate de que este constructor devuelva un *domain.IdentityProvider)
	emailProvider, err := domain.NewEmailProvider(providerID, user.ID(), user.Email(), cmd.Password)
	if err != nil {
		return nil, err
	}

	// 5. Generar el Token de Verificación (TTL 24h autogestionado por el dominio)
	verificationToken := domain.NewEmailVerificationToken(tokenValue, user.ID())

	// 6. Persistencia Atómica: Guardamos el usuario, su método de auth y el token
	// Definimos el rol por defecto para los registros automáticos por SSO
	initialRoles := []string{"INQUILINO"}
	err = uc.userRepo.Save(ctx, user, emailProvider, initialRoles)
	if err != nil {
		return nil, fmt.Errorf("error al guardar el usuario en la base de datos: %w", err)
	}

	err = uc.userRepo.SaveVerificationToken(ctx, verificationToken)
	if err != nil {
		return nil, fmt.Errorf("error al guardar el token de verificación: %w", err)
	}

	// 7. Disparar Evento de Dominio a NATS JetStream (Para que el bot de WhatsApp o Mailer lo envíe asincrónicamente)
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.created",
		UserID:    user.ID(),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"email":              user.Email(),
			"verification_token": verificationToken.Value(),
		},
	}

	// No bloqueamos el flujo si NATS falla, lo mandamos como un log o lo manejamos con Outbox si fuese crítico.
	// En este caso, para simplificar el arranque de Auth, lo tiramos directo al puerto del publisher.
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	// 8. Responder con el ID creado (Post-condición: Sin JWT todavía)
	return &RegisterUserResponse{
		UserID: user.ID(),
	}, nil
}
