package domain

import (
	"errors"
	"time"
)

// 1. Tipo de Índice de Ajuste (ICL, IPC, etc.)
type AdjustmentIndex string

const (
	IndexICL AdjustmentIndex = "ICL" // Índice de Contratos de Locación
	IndexIPC AdjustmentIndex = "IPC" // Índice de Precios al Consumidor
	IndexFix AdjustmentIndex = "FIX" // Sin ajuste (Monto fijo)
)

// 2. Periodicidad del ajuste en meses (ej: cada 4 meses, cada 6 meses)
type AdjustmentPeriod int

func NewAdjustmentPeriod(months int) (AdjustmentPeriod, error) {
	if months < 0 {
		return 0, errors.New("la periodicidad de ajuste no puede ser negativa")
	}
	return AdjustmentPeriod(months), nil
}

// 3. Timeline maneja la vigencia del contrato
type Timeline struct {
	startDate time.Time
	endDate   time.Time
}

func NewTimeline(start, end time.Time) (Timeline, error) {
	if start.IsZero() || end.IsZero() {
		return Timeline{}, errors.New("las fechas de vigencia no pueden estar vacías")
	}
	if end.Before(start) || end.Equal(start) {
		return Timeline{}, errors.New("la fecha de finalización debe ser posterior a la de inicio")
	}
	return Timeline{startDate: start, endDate: end}, nil
}

func (t Timeline) StartDate() time.Time { return t.startDate }
func (t Timeline) EndDate() time.Time   { return t.endDate }

// 4. RentAmount maneja el canon locativo y su moneda
type RentAmount struct {
	amount   float64
	currency string
}

func NewRentAmount(amount float64, currency string) (RentAmount, error) {
	if amount <= 0 {
		return RentAmount{}, errors.New("el monto del alquiler inicial debe ser mayor a cero")
	}
	if currency == "" {
		return RentAmount{}, errors.New("la moneda del contrato es requerida")
	}
	return RentAmount{amount: amount, currency: currency}, nil
}

func (r RentAmount) Amount() float64  { return r.amount }
func (r RentAmount) Currency() string { return r.currency }
