package domain

import "inmo-platform/shared/pkg/apperr"

// Location representa las coordenadas geográficas de la propiedad.
type Location struct {
	latitude  float64
	longitude float64
	address   string
}

func NewLocation(lat, lng float64, address string) (Location, error) {
	if address == "" {
		return Location{}, apperr.NewBadRequest("la dirección física no puede estar vacía", nil)
	}
	if lat < -90 || lat > 90 {
		return Location{}, apperr.NewBadRequest("la latitud debe estar entre -90 y 90 grados", nil)
	}
	if lng < -180 || lng > 180 {
		return Location{}, apperr.NewBadRequest("la longitud debe estar entre -180 y 180 grados", nil)
	}
	return Location{latitude: lat, longitude: lng, address: address}, nil
}

func (l Location) Latitude() float64  { return l.latitude }
func (l Location) Longitude() float64 { return l.longitude }
func (l Location) Address() string    { return l.address }
