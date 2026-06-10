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
	// ErrInvalidRole se devuelve cuando el frontend envía un rol que no existe en el sistema
	ErrInvalidRole = errors.New("el rol especificado no es válido")
)

// rolesValidos define el conjunto de roles que el sistema acepta en el registro.
// SSO auto-registra siempre como INTERESADO hasta que el usuario completa su perfil.
var rolesValidos = map[string]bool{
	"INQUILINO":   true,
	"PROPIETARIO": true,
	"AGENTE":      true,
	"PROVEEDOR":   true,
	"INTERESADO":  true,
}

// RegisterUserCommand transporta los datos de entrada desde el controlador HTTP
type RegisterUserCommand struct {
	Email    string
	Password string
	Role     string // ← NUEVO: requerido. Ej: "PROPIETARIO", "INQUILINO", "PROVEEDOR"
}

// RegisterUserResponse retorna la data resultante que pide el requerimiento (sin JWT)
type RegisterUserResponse struct {
	UserID string
	Role   string // ← devolvemos el rol asignado para confirmación al frontend
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
	// 0. Validar el rol ANTES de ir a la base de datos
	// Si el frontend no envía role o manda uno inválido, cortamos rápido con 400
	if cmd.Role == "" || !rolesValidos[cmd.Role] {
		return nil, ErrInvalidRole
	}

	// 1. Validar si el email ya existe en el sistema
	existingUser, err := uc.userRepo.FindByEmail(ctx, cmd.Email)
	if err != nil {
		return nil, fmt.Errorf("error al verificar existencia de email: %w", err)
	}

	if existingUser != nil {
		emailProvider, _ := uc.userRepo.FindProvider(ctx, existingUser.ID(), domain.ProviderEmail)
		if emailProvider != nil {
			return nil, ErrEmailAlreadyExists // 409 Conflict
		}
		return nil, errors.New("el email está registrado mediante SSO (Google/Meta). Intente iniciar sesión con ese proveedor")
	}

	// 2. Crear las IDs necesarias
	userID := uc.uuidGen()
	providerID := uc.uuidGen()
	tokenValue := uc.uuidGen()

	// 3. Instanciar el Agregado User (nace en PENDING_VERIFICATION)
	user, err := domain.NewUser(userID, cmd.Email)
	if err != nil {
		return nil, err
	}

	// 4. Instanciar el Provider EMAIL
	emailProvider, err := domain.NewEmailProvider(providerID, user.ID(), user.Email(), cmd.Password)
	if err != nil {
		return nil, err
	}

	// 5. Generar el Token de Verificación (TTL 24h)
	verificationToken := domain.NewEmailVerificationToken(tokenValue, user.ID())

	// 6. Persistencia Atómica con el rol que eligió el usuario en el frontend
	// Ya no hay hardcodeo — el rol viene del comando validado arriba
	initialRoles := []string{cmd.Role}
	err = uc.userRepo.Save(ctx, user, emailProvider, initialRoles)
	if err != nil {
		return nil, fmt.Errorf("error al guardar el usuario en la base de datos: %w", err)
	}

	err = uc.userRepo.SaveVerificationToken(ctx, verificationToken)
	if err != nil {
		return nil, fmt.Errorf("error al guardar el token de verificación: %w", err)
	}

	// 7. Disparar Evento de Dominio a NATS JetStream
	authEvent := ports.AuthEvent{
		EventID:   uc.uuidGen(),
		Name:      "auth.user.created",
		UserID:    user.ID(),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"email":              user.Email(),
			"role":               cmd.Role, // ← incluimos el rol en el evento para que otros módulos reaccionen
			"verification_token": verificationToken.Value(),
		},
	}
	_ = uc.eventPublisher.PublishEvent(ctx, authEvent)

	return &RegisterUserResponse{
		UserID: user.ID(),
		Role:   cmd.Role,
	}, nil
}
