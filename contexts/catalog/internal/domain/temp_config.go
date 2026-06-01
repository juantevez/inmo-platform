package domain

import "inmo.platform/shared/pkg/apperr"

// PricingRuleType clasifica el tipo de descuento aplicable.
type PricingRuleType string

const (
	PricingRuleWeekly  PricingRuleType = "weekly"
	PricingRuleMonthly PricingRuleType = "monthly"
)

// PricingRule define un descuento porcentual activado a partir de cierta cantidad de noches.
type PricingRule struct {
	Type        PricingRuleType `json:"type"`
	MinNights   int             `json:"min_nights"`
	DiscountPct float64         `json:"discount_pct"` // 0–100
}

// Amenity representa una comodidad de la propiedad con su categoría e ícono.
type Amenity struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Category string `json:"category"` // infrastructure | comfort | premium
	Icon     string `json:"icon,omitempty"`
}

// TempConfig agrupa toda la configuración específica de un alquiler temporario.
type TempConfig struct {
	amenities       []Amenity
	checkInTime     string // "14:00"
	checkOutTime    string // "10:00"
	minNights       int
	maxNights       int
	nightPrice      float64
	cleaningFee     float64
	securityDeposit float64
	pricingRules    []PricingRule
}

func NewTempConfig(
	amenities []Amenity,
	checkInTime, checkOutTime string,
	minNights, maxNights int,
	nightPrice, cleaningFee, securityDeposit float64,
	pricingRules []PricingRule,
) (TempConfig, error) {
	if minNights < 1 {
		return TempConfig{}, apperr.NewBadRequest("min_nights debe ser al menos 1", nil)
	}
	if maxNights < minNights {
		return TempConfig{}, apperr.NewBadRequest("max_nights no puede ser menor que min_nights", nil)
	}
	if nightPrice < 0 {
		return TempConfig{}, apperr.NewBadRequest("night_price no puede ser negativo", nil)
	}
	if cleaningFee < 0 || securityDeposit < 0 {
		return TempConfig{}, apperr.NewBadRequest("cleaning_fee y security_deposit no pueden ser negativos", nil)
	}
	if checkInTime == "" {
		checkInTime = "14:00"
	}
	if checkOutTime == "" {
		checkOutTime = "10:00"
	}
	if maxNights == 0 {
		maxNights = 90
	}
	return TempConfig{
		amenities:       amenities,
		checkInTime:     checkInTime,
		checkOutTime:    checkOutTime,
		minNights:       minNights,
		maxNights:       maxNights,
		nightPrice:      nightPrice,
		cleaningFee:     cleaningFee,
		securityDeposit: securityDeposit,
		pricingRules:    pricingRules,
	}, nil
}

func DefaultTempConfig() TempConfig {
	return TempConfig{
		checkInTime:  "14:00",
		checkOutTime: "10:00",
		minNights:    1,
		maxNights:    90,
	}
}

func (tc TempConfig) Amenities() []Amenity         { return tc.amenities }
func (tc TempConfig) CheckInTime() string           { return tc.checkInTime }
func (tc TempConfig) CheckOutTime() string          { return tc.checkOutTime }
func (tc TempConfig) MinNights() int                { return tc.minNights }
func (tc TempConfig) MaxNights() int                { return tc.maxNights }
func (tc TempConfig) NightPrice() float64           { return tc.nightPrice }
func (tc TempConfig) CleaningFee() float64          { return tc.cleaningFee }
func (tc TempConfig) SecurityDeposit() float64      { return tc.securityDeposit }
func (tc TempConfig) PricingRules() []PricingRule   { return tc.pricingRules }

// ApplyDiscount calcula el porcentaje de descuento que aplica según la cantidad de noches.
// Retorna el mayor descuento elegible (weekly vs monthly).
func (tc TempConfig) ApplyDiscount(nights int) float64 {
	best := 0.0
	for _, rule := range tc.pricingRules {
		if nights >= rule.MinNights && rule.DiscountPct > best {
			best = rule.DiscountPct
		}
	}
	return best
}
