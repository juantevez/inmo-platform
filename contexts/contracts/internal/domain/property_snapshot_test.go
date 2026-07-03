package domain_test

import (
	"testing"

	"inmo.platform/contexts/contracts/internal/domain"
)

func TestApplyDiscount_SinReglas_RetornaCero(t *testing.T) {
	s := domain.PropertySnapshot{}

	if got := s.ApplyDiscount(30); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestApplyDiscount_NochesInsuficientes_NoAplicaNingunaRegla(t *testing.T) {
	s := domain.PropertySnapshot{
		PricingRules: []domain.PricingRule{
			{Type: "weekly", MinNights: 7, DiscountPct: 10},
			{Type: "monthly", MinNights: 30, DiscountPct: 20},
		},
	}

	if got := s.ApplyDiscount(5); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestApplyDiscount_NochesIgualAlMinimo_AplicaLaRegla(t *testing.T) {
	s := domain.PropertySnapshot{
		PricingRules: []domain.PricingRule{
			{Type: "weekly", MinNights: 7, DiscountPct: 10},
		},
	}

	if got := s.ApplyDiscount(7); got != 10 {
		t.Fatalf("got %v, want 10 (el límite es inclusivo: nights >= MinNights)", got)
	}
}

func TestApplyDiscount_VariasReglasAplicables_RetornaElMayorDescuento(t *testing.T) {
	s := domain.PropertySnapshot{
		PricingRules: []domain.PricingRule{
			{Type: "weekly", MinNights: 7, DiscountPct: 10},
			{Type: "monthly", MinNights: 30, DiscountPct: 20},
			{Type: "biweekly", MinNights: 14, DiscountPct: 15},
		},
	}

	if got := s.ApplyDiscount(30); got != 20 {
		t.Fatalf("got %v, want 20 (el mejor descuento aplicable para 30 noches)", got)
	}
	if got := s.ApplyDiscount(14); got != 15 {
		t.Fatalf("got %v, want 15 (30 noches aún no aplica, pero sí 14 y 7)", got)
	}
}

func TestApplyDiscount_ReglaConDescuentoCero_NoDesplazaAlMejorVigente(t *testing.T) {
	s := domain.PropertySnapshot{
		PricingRules: []domain.PricingRule{
			{Type: "weekly", MinNights: 7, DiscountPct: 10},
			{Type: "promo-inactiva", MinNights: 1, DiscountPct: 0},
		},
	}

	if got := s.ApplyDiscount(10); got != 10 {
		t.Fatalf("got %v, want 10", got)
	}
}
