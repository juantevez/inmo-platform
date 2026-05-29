package domain

import (
	"inmo.platform/shared/pkg/apperr"
)

type Currency string

const (
	ARS Currency = "ARS"
	USD Currency = "USD"
)

// Price es un Value Object inmutable.
type Price struct {
	amount   float64
	currency Currency
}

func NewPrice(amount float64, currency Currency) (Price, error) {
	if amount <= 0 {
		return Price{}, apperr.NewBadRequest("el precio debe ser un valor positivo mayor a cero", nil)
	}
	if currency != ARS && currency != USD {
		return Price{}, apperr.NewBadRequest("la moneda especificada no es válida (solo ARS o USD)", nil)
	}
	return Price{amount: amount, currency: currency}, nil
}

func (p Price) Amount() float64    { return p.amount }
func (p Price) Currency() Currency { return p.currency }
