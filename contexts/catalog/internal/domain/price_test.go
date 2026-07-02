package domain_test

import (
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── NewPrice ───────────────────────────────────────────────────────────────

func TestNewPrice_MontoCero_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPrice(0, domain.USD)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPrice: got %v, want AppError BadRequest", err)
	}
}

func TestNewPrice_MontoNegativo_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPrice(-100, domain.USD)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPrice: got %v, want AppError BadRequest", err)
	}
}

func TestNewPrice_MonedaInvalida_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewPrice(1000, domain.Currency("EUR"))

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewPrice: got %v, want AppError BadRequest", err)
	}
}

func TestNewPrice_ValidacionDeMontoTienePrioridadSobreMoneda(t *testing.T) {
	// Con monto inválido Y moneda inválida, el primer chequeo (monto) determina el mensaje.
	_, err := domain.NewPrice(-1, domain.Currency("EUR"))

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("NewPrice: got %v, want *apperr.AppError", err)
	}
	if appErr.Message != "el precio debe ser un valor positivo mayor a cero" {
		t.Errorf("Message: got %q, want el mensaje de monto inválido primero", appErr.Message)
	}
}

func TestNewPrice_HappyPath_ARS(t *testing.T) {
	price, err := domain.NewPrice(50000, domain.ARS)

	if err != nil {
		t.Fatalf("NewPrice: error inesperado: %v", err)
	}
	if price.Amount() != 50000 {
		t.Errorf("Amount: got %v, want %v", price.Amount(), 50000)
	}
	if price.Currency() != domain.ARS {
		t.Errorf("Currency: got %q, want %q", price.Currency(), domain.ARS)
	}
}

func TestNewPrice_HappyPath_USD(t *testing.T) {
	price, err := domain.NewPrice(1500.50, domain.USD)

	if err != nil {
		t.Fatalf("NewPrice: error inesperado: %v", err)
	}
	if price.Amount() != 1500.50 {
		t.Errorf("Amount: got %v, want %v", price.Amount(), 1500.50)
	}
	if price.Currency() != domain.USD {
		t.Errorf("Currency: got %q, want %q", price.Currency(), domain.USD)
	}
}
