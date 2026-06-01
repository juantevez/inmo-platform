package domain

import "time"

// PricingRule refleja la misma estructura que publica Catálogo en el evento property.published.
type PricingRule struct {
	Type        string  `json:"type"`
	MinNights   int     `json:"min_nights"`
	DiscountPct float64 `json:"discount_pct"`
}

// PropertySnapshot es el mirror local de los datos de Catálogo que Contratos necesita
// para calcular precios y validar restricciones sin llamar a Catálogo en tiempo real.
type PropertySnapshot struct {
	PropertyID      string
	OwnerID         string
	OperationType   string
	NightPrice      float64
	CleaningFee     float64
	SecurityDeposit float64
	MinNights       int
	MaxNights       int
	CheckInTime     string
	CheckOutTime    string
	PricingRules    []PricingRule
	UpdatedAt       time.Time
}

// ApplyDiscount retorna el mejor descuento porcentual aplicable según las noches.
func (s PropertySnapshot) ApplyDiscount(nights int) float64 {
	best := 0.0
	for _, r := range s.PricingRules {
		if nights >= r.MinNights && r.DiscountPct > best {
			best = r.DiscountPct
		}
	}
	return best
}
