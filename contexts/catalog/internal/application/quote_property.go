package application

import (
	"context"
	"fmt"
	"time"

	"inmo.platform/contexts/catalog/internal/ports"
	"inmo.platform/shared/pkg/apperr"
)

type QuotePropertyUseCase struct {
	propertyRepo ports.PropertyRepository
	blockedRepo  ports.BlockedDatesRepository
}

func NewQuotePropertyUseCase(propertyRepo ports.PropertyRepository, blockedRepo ports.BlockedDatesRepository) *QuotePropertyUseCase {
	return &QuotePropertyUseCase{propertyRepo: propertyRepo, blockedRepo: blockedRepo}
}

type QuoteCommand struct {
	PropertyID   string
	CheckInDate  time.Time
	CheckOutDate time.Time
}

type QuoteLineItem struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

type QuoteResponse struct {
	PropertyID      string          `json:"property_id"`
	CheckInDate     string          `json:"check_in_date"`
	CheckOutDate    string          `json:"check_out_date"`
	CheckInTime     string          `json:"check_in_time"`
	CheckOutTime    string          `json:"check_out_time"`
	Nights          int             `json:"nights"`
	NightPrice      float64         `json:"night_price"`
	Subtotal        float64         `json:"subtotal"`
	DiscountPct     float64         `json:"discount_pct"`
	DiscountAmount  float64         `json:"discount_amount"`
	CleaningFee     float64         `json:"cleaning_fee"`
	SecurityDeposit float64         `json:"security_deposit"`
	Total           float64         `json:"total"`
	Breakdown       []QuoteLineItem `json:"breakdown"`
}

func (uc *QuotePropertyUseCase) Execute(ctx context.Context, cmd QuoteCommand) (*QuoteResponse, error) {
	if cmd.CheckInDate.IsZero() || cmd.CheckOutDate.IsZero() {
		return nil, apperr.NewBadRequest("check_in_date y check_out_date son obligatorios", nil)
	}
	if !cmd.CheckOutDate.After(cmd.CheckInDate) {
		return nil, apperr.NewBadRequest("check_out_date debe ser posterior a check_in_date", nil)
	}

	property, err := uc.propertyRepo.FindByID(ctx, cmd.PropertyID)
	if err != nil {
		return nil, err
	}
	if property == nil {
		return nil, apperr.NewNotFound("propiedad no encontrada", nil)
	}

	tc := property.TempConfig()
	nights := int(cmd.CheckOutDate.Sub(cmd.CheckInDate).Hours() / 24)

	if nights < tc.MinNights() {
		return nil, apperr.NewBadRequest(
			"la estadía mínima para esta propiedad es de "+itoa(tc.MinNights())+" noches", nil,
		)
	}
	if tc.MaxNights() > 0 && nights > tc.MaxNights() {
		return nil, apperr.NewBadRequest(
			"la estadía máxima para esta propiedad es de "+itoa(tc.MaxNights())+" noches", nil,
		)
	}

	// Verificar disponibilidad
	hasConflict, err := uc.blockedRepo.HasOverlap(ctx, cmd.PropertyID, cmd.CheckInDate, cmd.CheckOutDate)
	if err != nil {
		return nil, err
	}
	if hasConflict {
		return nil, apperr.NewPreconditionFailed("las fechas seleccionadas no están disponibles", nil)
	}

	// Calcular precio
	nightPrice := tc.NightPrice()
	discountPct := tc.ApplyDiscount(nights)
	subtotalBase := nightPrice * float64(nights)
	discountAmt := subtotalBase * discountPct / 100
	subtotal := subtotalBase - discountAmt
	total := subtotal + tc.CleaningFee()

	breakdown := []QuoteLineItem{
		{Description: formatf(nightPrice) + " x " + itoa(nights) + " noches", Amount: subtotalBase},
	}
	if discountAmt > 0 {
		breakdown = append(breakdown, QuoteLineItem{
			Description: "Descuento (" + formatf(discountPct) + "%)",
			Amount:      -discountAmt,
		})
	}
	if tc.CleaningFee() > 0 {
		breakdown = append(breakdown, QuoteLineItem{Description: "Tarifa de limpieza", Amount: tc.CleaningFee()})
	}
	if tc.SecurityDeposit() > 0 {
		breakdown = append(breakdown, QuoteLineItem{Description: "Depósito en garantía (reintegrable)", Amount: tc.SecurityDeposit()})
	}

	return &QuoteResponse{
		PropertyID:      cmd.PropertyID,
		CheckInDate:     cmd.CheckInDate.Format("2006-01-02"),
		CheckOutDate:    cmd.CheckOutDate.Format("2006-01-02"),
		CheckInTime:     tc.CheckInTime(),
		CheckOutTime:    tc.CheckOutTime(),
		Nights:          nights,
		NightPrice:      nightPrice,
		Subtotal:        subtotalBase,
		DiscountPct:     discountPct,
		DiscountAmount:  discountAmt,
		CleaningFee:     tc.CleaningFee(),
		SecurityDeposit: tc.SecurityDeposit(),
		Total:           total,
		Breakdown:       breakdown,
	}, nil
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func formatf(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
