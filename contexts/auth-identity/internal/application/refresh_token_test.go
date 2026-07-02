package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/auth-identity/internal/application"
)

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestRefreshExecute_TokenVacio_RetornaErrInvalidRefreshToken(t *testing.T) {
	tokenRepo := &fakeTokenRepo{}
	uc := application.NewRefreshTokenUseCase(tokenRepo, &fakeTokenService{})

	_, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: ""})

	if !errors.Is(err, application.ErrInvalidRefreshToken) {
		t.Fatalf("Execute: got %v, want ErrInvalidRefreshToken", err)
	}
	// No debería haber ido ni a Redis con un token vacío
	if tokenRepo.deleteRefreshCalled {
		t.Error("DeleteRefreshToken: no debería llamarse cuando el token de entrada está vacío")
	}
}

func TestRefreshExecute_TokenNoEncontradoEnRedis_RetornaErrInvalidRefreshToken(t *testing.T) {
	// GetRefreshToken puede fallar tanto porque el token no existe como por un error real de Redis;
	// el caso de uso homogeniza ambos casos en el mismo error genérico de seguridad.
	tokenRepo := &fakeTokenRepo{
		getRefreshFn: func(ctx context.Context, tokenID string) (string, error) {
			return "", errors.New("token no encontrado")
		},
	}
	uc := application.NewRefreshTokenUseCase(tokenRepo, &fakeTokenService{})

	_, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: "old-refresh-token"})

	if !errors.Is(err, application.ErrInvalidRefreshToken) {
		t.Fatalf("Execute: got %v, want ErrInvalidRefreshToken", err)
	}
	if tokenRepo.deleteRefreshCalled {
		t.Error("DeleteRefreshToken: no debería llamarse si el token nunca se encontró")
	}
}

func TestRefreshExecute_ErrorGenerandoAccessToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("clave de firma inválida")
	tokenRepo := &fakeTokenRepo{
		getRefreshFn: func(ctx context.Context, tokenID string) (string, error) { return "user-1", nil },
	}
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) { return "", boom },
	}
	uc := application.NewRefreshTokenUseCase(tokenRepo, tokenSvc)

	_, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: "old-refresh-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
	// La rotación (RTR) ya debe haber quemado el token viejo ANTES de intentar firmar el nuevo
	if !tokenRepo.deleteRefreshCalled || tokenRepo.deletedRefreshToken != "old-refresh-token" {
		t.Errorf("DeleteRefreshToken: got called=%v token=%q, want called=true token=%q",
			tokenRepo.deleteRefreshCalled, tokenRepo.deletedRefreshToken, "old-refresh-token")
	}
}

func TestRefreshExecute_ErrorGenerandoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("fallo generando refresh token")
	tokenRepo := &fakeTokenRepo{
		getRefreshFn: func(ctx context.Context, tokenID string) (string, error) { return "user-1", nil },
	}
	tokenSvc := &fakeTokenService{
		generateRefreshFn: func() (string, error) { return "", boom },
	}
	uc := application.NewRefreshTokenUseCase(tokenRepo, tokenSvc)

	_, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: "old-refresh-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestRefreshExecute_ErrorPersistiendoNuevoRefreshToken_RetornaErrorEnvuelto(t *testing.T) {
	boom := errors.New("redis no disponible")
	tokenRepo := &fakeTokenRepo{
		getRefreshFn: func(ctx context.Context, tokenID string) (string, error) { return "user-1", nil },
		setRefreshFn: func(ctx context.Context, tokenID string, userID string, ttl time.Duration) error { return boom },
	}
	uc := application.NewRefreshTokenUseCase(tokenRepo, &fakeTokenService{})

	_, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: "old-refresh-token"})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestRefreshExecute_HappyPath_RotaTokensYPersisteNuevaSesion(t *testing.T) {
	tokenRepo := &fakeTokenRepo{
		getRefreshFn: func(ctx context.Context, tokenID string) (string, error) {
			if tokenID != "old-refresh-token" {
				t.Errorf("GetRefreshToken: got %q, want %q", tokenID, "old-refresh-token")
			}
			return "user-42", nil
		},
	}
	var capturedUserID string
	var capturedRoles []string
	tokenSvc := &fakeTokenService{
		generateAccessFn: func(userID string, roles []string) (string, error) {
			capturedUserID = userID
			capturedRoles = roles
			return "new-access-token", nil
		},
		generateRefreshFn: func() (string, error) { return "new-refresh-token", nil },
	}
	uc := application.NewRefreshTokenUseCase(tokenRepo, tokenSvc)

	resp, err := uc.Execute(context.Background(), application.RefreshTokenCommand{RefreshToken: "old-refresh-token"})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.AccessToken != "new-access-token" || resp.RefreshToken != "new-refresh-token" {
		t.Errorf("Response: got %+v, want tokens rotados", resp)
	}

	// El access token nuevo se firma con el userID resuelto desde Redis
	if capturedUserID != "user-42" {
		t.Errorf("GenerateAccessToken userID: got %q, want %q", capturedUserID, "user-42")
	}
	if len(capturedRoles) != 1 || capturedRoles[0] != "INQUILINO" {
		t.Errorf("GenerateAccessToken roles: got %v, want [INQUILINO]", capturedRoles)
	}

	// RTR: el token viejo se quema
	if !tokenRepo.deleteRefreshCalled || tokenRepo.deletedRefreshToken != "old-refresh-token" {
		t.Errorf("DeleteRefreshToken: got called=%v token=%q, want called=true token=%q",
			tokenRepo.deleteRefreshCalled, tokenRepo.deletedRefreshToken, "old-refresh-token")
	}

	// El nuevo refresh token se persiste con TTL de 7 días bajo el mismo userID
	if !tokenRepo.setRefreshCalled || tokenRepo.setRefreshUserID != "user-42" || tokenRepo.setRefreshTTL != 7*24*time.Hour {
		t.Errorf("SetRefreshToken: got called=%v userID=%q ttl=%v",
			tokenRepo.setRefreshCalled, tokenRepo.setRefreshUserID, tokenRepo.setRefreshTTL)
	}
}
