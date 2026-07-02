package domain_test

import (
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── NewTempConfig ──────────────────────────────────────────────────────────

func TestNewTempConfig_MinNightsMenorAUno_RetornaBadRequest(t *testing.T) {
	cases := []int{0, -1}
	for _, minNights := range cases {
		_, err := domain.NewTempConfig(nil, "", "", minNights, 30, 50, 0, 0, nil)
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
			t.Errorf("NewTempConfig(minNights=%d): got %v, want AppError BadRequest", minNights, err)
		}
	}
}

func TestNewTempConfig_MaxNightsMenorQueMinNights_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewTempConfig(nil, "", "", 10, 5, 50, 0, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewTempConfig: got %v, want AppError BadRequest (max < min)", err)
	}
}

// BUG DOCUMENTADO: la intención del código es que max_nights=0 signifique
// "no especificado" y se autocomplete a 90 (ver el "if maxNights == 0" al final
// de la función). Pero esa validación de default corre DESPUÉS del chequeo
// "maxNights < minNights", así que con cualquier minNights >= 1 (el único valor
// permitido, ya que minNights < 1 se rechaza antes), 0 < minNights siempre es
// verdadero y la función devuelve un error ANTES de llegar a aplicar el default.
// En la práctica: un caller que omite max_nights (el zero-value típico al
// decodificar JSON) nunca puede crear una propiedad TEMP — la línea de default
// es código muerto e inalcanzable.
func TestNewTempConfig_MaxNightsOmitido_ActualmenteFallaEnVezDeUsarElDefault(t *testing.T) {
	_, err := domain.NewTempConfig(nil, "", "", 1, 0, 50, 0, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewTempConfig(minNights=1, maxNights=0): got %v, want AppError BadRequest — "+
			"comportamiento actual (posible bug: se esperaría que 0 dispare el default a 90)", err)
	}
}

func TestNewTempConfig_NightPriceNegativo_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewTempConfig(nil, "", "", 1, 30, -1, 0, 0, nil)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("NewTempConfig: got %v, want AppError BadRequest (night_price negativo)", err)
	}
}

func TestNewTempConfig_CleaningFeeYSecurityDepositNegativos_RetornaBadRequest(t *testing.T) {
	cases := []struct {
		name            string
		cleaningFee     float64
		securityDeposit float64
	}{
		{"cleaning_fee negativo", -1, 0},
		{"security_deposit negativo", 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, tc.cleaningFee, tc.securityDeposit, nil)
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
				t.Fatalf("NewTempConfig: got %v, want AppError BadRequest", err)
			}
		})
	}
}

func TestNewTempConfig_HorariosVacios_UsanDefaults(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)

	if err != nil {
		t.Fatalf("NewTempConfig: error inesperado: %v", err)
	}
	if tc.CheckInTime() != "14:00" {
		t.Errorf("CheckInTime: got %q, want %q", tc.CheckInTime(), "14:00")
	}
	if tc.CheckOutTime() != "10:00" {
		t.Errorf("CheckOutTime: got %q, want %q", tc.CheckOutTime(), "10:00")
	}
}

func TestNewTempConfig_HorariosExplicitos_SePreservan(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "16:00", "12:00", 1, 30, 50, 0, 0, nil)

	if err != nil {
		t.Fatalf("NewTempConfig: error inesperado: %v", err)
	}
	if tc.CheckInTime() != "16:00" || tc.CheckOutTime() != "12:00" {
		t.Errorf("got CheckInTime=%q CheckOutTime=%q", tc.CheckInTime(), tc.CheckOutTime())
	}
}

func TestNewTempConfig_MaxNightsExplicitoNoSeSobreescribe(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 2, 15, 50, 0, 0, nil)

	if err != nil {
		t.Fatalf("NewTempConfig: error inesperado: %v", err)
	}
	if tc.MaxNights() != 15 {
		t.Errorf("MaxNights: got %d, want 15 (no debería aplicarse el default)", tc.MaxNights())
	}
}

func TestNewTempConfig_HappyPath_PreservaAmenitiesYPricingRules(t *testing.T) {
	amenities := []domain.Amenity{{Key: "wifi", Label: "WiFi", Category: "infrastructure"}}
	rules := []domain.PricingRule{{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10}}

	tc, err := domain.NewTempConfig(amenities, "15:00", "11:00", 2, 30, 50, 20, 100, rules)

	if err != nil {
		t.Fatalf("NewTempConfig: error inesperado: %v", err)
	}
	if len(tc.Amenities()) != 1 || tc.Amenities()[0].Key != "wifi" {
		t.Errorf("Amenities: got %+v", tc.Amenities())
	}
	if len(tc.PricingRules()) != 1 || tc.PricingRules()[0].DiscountPct != 10 {
		t.Errorf("PricingRules: got %+v", tc.PricingRules())
	}
	if tc.MinNights() != 2 || tc.MaxNights() != 30 || tc.NightPrice() != 50 || tc.CleaningFee() != 20 || tc.SecurityDeposit() != 100 {
		t.Errorf("TempConfig: got %+v", tc)
	}
}

// ─── DefaultTempConfig ──────────────────────────────────────────────────────

func TestDefaultTempConfig(t *testing.T) {
	tc := domain.DefaultTempConfig()

	if tc.CheckInTime() != "14:00" || tc.CheckOutTime() != "10:00" {
		t.Errorf("horarios: got CheckInTime=%q CheckOutTime=%q", tc.CheckInTime(), tc.CheckOutTime())
	}
	if tc.MinNights() != 1 || tc.MaxNights() != 90 {
		t.Errorf("noches: got MinNights=%d MaxNights=%d", tc.MinNights(), tc.MaxNights())
	}
	if tc.NightPrice() != 0 || tc.CleaningFee() != 0 || tc.SecurityDeposit() != 0 {
		t.Errorf("montos: got NightPrice=%v CleaningFee=%v SecurityDeposit=%v", tc.NightPrice(), tc.CleaningFee(), tc.SecurityDeposit())
	}
	if tc.Amenities() != nil || tc.PricingRules() != nil {
		t.Errorf("Amenities/PricingRules: got %v / %v, want nil", tc.Amenities(), tc.PricingRules())
	}
}

// ─── ApplyDiscount ──────────────────────────────────────────────────────────

func TestApplyDiscount_SinReglas_RetornaCero(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	if got := tc.ApplyDiscount(10); got != 0 {
		t.Errorf("ApplyDiscount: got %v, want 0", got)
	}
}

func TestApplyDiscount_NochesInsuficientes_RetornaCero(t *testing.T) {
	rules := []domain.PricingRule{{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10}}
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, rules)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	if got := tc.ApplyDiscount(5); got != 0 {
		t.Errorf("ApplyDiscount(5): got %v, want 0 (por debajo del mínimo de la regla)", got)
	}
}

func TestApplyDiscount_ExactamenteEnElLimite_Aplica(t *testing.T) {
	// El chequeo es "nights >= rule.MinNights" — el límite es inclusivo.
	rules := []domain.PricingRule{{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10}}
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, rules)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	if got := tc.ApplyDiscount(7); got != 10 {
		t.Errorf("ApplyDiscount(7): got %v, want 10 (7 == MinNights, límite inclusivo)", got)
	}
}

func TestApplyDiscount_MultiplesReglasElegibles_RetornaElMayorDescuento(t *testing.T) {
	rules := []domain.PricingRule{
		{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10},
		{Type: domain.PricingRuleMonthly, MinNights: 30, DiscountPct: 20},
	}
	tc, err := domain.NewTempConfig(nil, "", "", 1, 60, 50, 0, 0, rules)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}

	// 30 noches: ambas reglas son elegibles (30>=7 y 30>=30) — debe ganar la de mayor descuento (20%).
	if got := tc.ApplyDiscount(30); got != 20 {
		t.Errorf("ApplyDiscount(30): got %v, want 20 (la mejor entre weekly y monthly)", got)
	}
	// 10 noches: solo weekly es elegible.
	if got := tc.ApplyDiscount(10); got != 10 {
		t.Errorf("ApplyDiscount(10): got %v, want 10 (solo weekly aplica)", got)
	}
	// 3 noches: ninguna regla aplica.
	if got := tc.ApplyDiscount(3); got != 0 {
		t.Errorf("ApplyDiscount(3): got %v, want 0", got)
	}
}
