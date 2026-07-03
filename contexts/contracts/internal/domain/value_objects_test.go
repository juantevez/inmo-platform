package domain_test

import (
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/domain"
)

// ─── AdjustmentPeriod ───────────────────────────────────────────────────────

func TestNewAdjustmentPeriod_MesesNegativos_RetornaError(t *testing.T) {
	if _, err := domain.NewAdjustmentPeriod(-1); err == nil {
		t.Fatal("esperaba error para una periodicidad negativa")
	}
}

func TestNewAdjustmentPeriod_MesesCeroOPositivos_Ok(t *testing.T) {
	tests := []int{0, 1, 4, 12}

	for _, months := range tests {
		p, err := domain.NewAdjustmentPeriod(months)
		if err != nil {
			t.Fatalf("meses=%d: no esperaba error: %v", months, err)
		}
		if int(p) != months {
			t.Fatalf("meses=%d: got %d", months, int(p))
		}
	}
}

// ─── Timeline ───────────────────────────────────────────────────────────────

func TestNewTimeline_FechasVacias_RetornaError(t *testing.T) {
	valid := time.Now()

	tests := []struct {
		name       string
		start, end time.Time
	}{
		{"start vacío", time.Time{}, valid},
		{"end vacío", valid, time.Time{}},
		{"ambos vacíos", time.Time{}, time.Time{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewTimeline(tc.start, tc.end); err == nil {
				t.Fatal("esperaba error")
			}
		})
	}
}

func TestNewTimeline_EndNoPosteriorAStart_RetornaError(t *testing.T) {
	start := time.Now()

	tests := []struct {
		name string
		end  time.Time
	}{
		{"end igual a start", start},
		{"end anterior a start", start.Add(-24 * time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewTimeline(start, tc.end); err == nil {
				t.Fatal("esperaba error")
			}
		})
	}
}

func TestNewTimeline_FechasValidas_MapeaStartYEnd(t *testing.T) {
	start := time.Now()
	end := start.Add(365 * 24 * time.Hour)

	tl, err := domain.NewTimeline(start, end)
	if err != nil {
		t.Fatalf("NewTimeline: %v", err)
	}
	if !tl.StartDate().Equal(start) {
		t.Fatalf("StartDate: got %v, want %v", tl.StartDate(), start)
	}
	if !tl.EndDate().Equal(end) {
		t.Fatalf("EndDate: got %v, want %v", tl.EndDate(), end)
	}
}

// ─── RentAmount ─────────────────────────────────────────────────────────────

func TestNewRentAmount_MontoInvalido_RetornaError(t *testing.T) {
	tests := []float64{0, -1, -100.5}

	for _, amount := range tests {
		if _, err := domain.NewRentAmount(amount, "ARS"); err == nil {
			t.Fatalf("monto=%v: esperaba error", amount)
		}
	}
}

func TestNewRentAmount_MonedaVacia_RetornaError(t *testing.T) {
	if _, err := domain.NewRentAmount(1000, ""); err == nil {
		t.Fatal("esperaba error por moneda vacía")
	}
}

func TestNewRentAmount_ValoresValidos_MapeaAmountYCurrency(t *testing.T) {
	r, err := domain.NewRentAmount(150000, "USD")
	if err != nil {
		t.Fatalf("NewRentAmount: %v", err)
	}
	if r.Amount() != 150000 {
		t.Fatalf("Amount: got %v, want 150000", r.Amount())
	}
	if r.Currency() != "USD" {
		t.Fatalf("Currency: got %q, want %q", r.Currency(), "USD")
	}
}
