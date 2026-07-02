package domain_test

import (
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── NewLocation ────────────────────────────────────────────────────────────

func TestNewLocation_DireccionVacia_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewLocation(-34.6, -58.4, "")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewLocation: got %v, want AppError BadRequest", err)
	}
}

func TestNewLocation_LatitudFueraDeRango_RetornaBadRequest(t *testing.T) {
	cases := []struct {
		name string
		lat  float64
	}{
		{"menor a -90", -90.1},
		{"mayor a 90", 90.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewLocation(tc.lat, 0, "Calle Falsa 123")
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
				t.Fatalf("NewLocation(lat=%v): got %v, want AppError BadRequest", tc.lat, err)
			}
		})
	}
}

func TestNewLocation_LongitudFueraDeRango_RetornaBadRequest(t *testing.T) {
	cases := []struct {
		name string
		lng  float64
	}{
		{"menor a -180", -180.1},
		{"mayor a 180", 180.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewLocation(0, tc.lng, "Calle Falsa 123")
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
				t.Fatalf("NewLocation(lng=%v): got %v, want AppError BadRequest", tc.lng, err)
			}
		})
	}
}

func TestNewLocation_LimitesExactosSonValidos(t *testing.T) {
	// -90/90 y -180/180 son los bordes inclusivos del rango válido.
	cases := []struct {
		name string
		lat  float64
		lng  float64
	}{
		{"lat=-90", -90, 0},
		{"lat=90", 90, 0},
		{"lng=-180", 0, -180},
		{"lng=180", 0, 180},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := domain.NewLocation(tc.lat, tc.lng, "Calle Falsa 123")
			if err != nil {
				t.Fatalf("NewLocation(lat=%v, lng=%v): error inesperado: %v", tc.lat, tc.lng, err)
			}
			if loc.Latitude() != tc.lat || loc.Longitude() != tc.lng {
				t.Errorf("got lat=%v lng=%v, want lat=%v lng=%v", loc.Latitude(), loc.Longitude(), tc.lat, tc.lng)
			}
		})
	}
}

func TestNewLocation_ValidacionDeDireccionTienePrioridad(t *testing.T) {
	// Con dirección vacía Y coordenadas inválidas, el primer chequeo (dirección)
	// es el que determina el mensaje de error devuelto.
	_, err := domain.NewLocation(999, 999, "")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("NewLocation: got %v, want *apperr.AppError", err)
	}
	if appErr.Message != "la dirección física no puede estar vacía" {
		t.Errorf("Message: got %q, want el mensaje de dirección vacía primero", appErr.Message)
	}
}

func TestNewLocation_HappyPath(t *testing.T) {
	loc, err := domain.NewLocation(-34.603722, -58.381592, "Av. de Mayo 1000, CABA")

	if err != nil {
		t.Fatalf("NewLocation: error inesperado: %v", err)
	}
	if loc.Latitude() != -34.603722 {
		t.Errorf("Latitude: got %v, want %v", loc.Latitude(), -34.603722)
	}
	if loc.Longitude() != -58.381592 {
		t.Errorf("Longitude: got %v, want %v", loc.Longitude(), -58.381592)
	}
	if loc.Address() != "Av. de Mayo 1000, CABA" {
		t.Errorf("Address: got %q, want %q", loc.Address(), "Av. de Mayo 1000, CABA")
	}
}
