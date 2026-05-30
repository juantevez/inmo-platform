package domain

import (
	"errors"
	"time"
)

var (
	ErrTokenExpired     = errors.New("el token ha expirado")
	ErrTokenAlreadyUsed = errors.New("el token ya fue utilizado")
)

type TokenType string

const (
	TypeEmailVerification TokenType = "EMAIL_VERIFICATION"
	TypePhoneOTP          TokenType = "PHONE_OTP"
)

// VerificationToken representa un código/token de verificación temporal
type VerificationToken struct {
	token     string
	tokenType TokenType
	userID    string
	expiresAt time.Time
	usedAt    *time.Time
}

// NewEmailVerificationToken genera un token UUID para verificación de email con TTL de 24h (UC-01)
func NewEmailVerificationToken(tokenValue, userID string) *VerificationToken {
	return &VerificationToken{
		token:     tokenValue,
		tokenType: TypeEmailVerification,
		userID:    userID,
		expiresAt: time.Now().Add(24 * time.Hour),
	}
}

// NewPhoneOTP genera un token numérico de 6 dígitos con TTL de 10 minutos (UC-07)
func NewPhoneOTP(otpValue, userID string) *VerificationToken {
	return &VerificationToken{
		token:     otpValue,
		tokenType: TypePhoneOTP,
		userID:    userID,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
}

// Validate comprueba si el token es apto para ser procesado
func (t *VerificationToken) Validate() error {
	if t.usedAt != nil {
		return ErrTokenAlreadyUsed
	}
	if time.Now().After(t.expiresAt) {
		return ErrTokenExpired
	}
	return nil
}

// Use marca el token como consumido (UC-02 / UC-07)
func (t *VerificationToken) Use() error {
	if err := t.Validate(); err != nil {
		return err
	}
	now := time.Now()
	t.usedAt = &now
	return nil
}

// ReconstructVerificationToken hidrata el token desde la base de datos relacional
func ReconstructVerificationToken(token string, tType TokenType, userID string, expiresAt time.Time, usedAt *time.Time) *VerificationToken {
	return &VerificationToken{
		token:     token,
		tokenType: tType,
		userID:    userID,
		expiresAt: expiresAt,
		usedAt:    usedAt,
	}
}

// Getters
func (t *VerificationToken) Value() string        { return t.token }
func (t *VerificationToken) Type() TokenType      { return t.tokenType }
func (t *VerificationToken) UserID() string       { return t.userID }
func (t *VerificationToken) ExpiresAt() time.Time { return t.expiresAt }
func (t *VerificationToken) UsedAt() *time.Time   { return t.usedAt }
