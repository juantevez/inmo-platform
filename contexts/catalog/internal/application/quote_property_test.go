package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeBlockedDatesRepo struct {
	hasOverlap   bool
	overlapErr   error
	calledStart  time.Time
	calledEnd    time.Time
	calledPropID string
}

func (f *fakeBlockedDatesRepo) HasOverlap(ctx context.Context, propertyID string, start, end time.Time) (bool, error) {
	f.calledPropID = propertyID
	f.calledStart = start
	f.calledEnd = end
	if f.overlapErr != nil {
		return false, f.overlapErr
	}
	return f.hasOverlap, nil
}
func (f *fakeBlockedDatesRepo) Block(ctx context.Context, propertyID, reservationID, reason string, start, end time.Time) error {
	return errors.New("Block: no debería invocarse en estos tests")
}
func (f *fakeBlockedDatesRepo) Unblock(ctx context.Context, reservationID string) error {
	return errors.New("Unblock: no debería invocarse en estos tests")
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// buildTempProperty crea una propiedad TEMP con el TempConfig indicado, lista
// para cotizar. tc puede construirse con domain.NewTempConfig en cada test.
func buildTempProperty(t *testing.T, id string, tc domain.TempConfig) *domain.Property {
	t.Helper()
	price, err := domain.NewPrice(100, domain.USD)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	loc, err := domain.NewLocation(-34.6, -58.4, "Calle Falsa 123")
	if err != nil {
		t.Fatalf("NewLocation: %v", err)
	}
	p, err := domain.NewProperty(id, "owner-1", "Depto temporario", "desc", price, loc, domain.OperationTemp, domain.PetPolicyAllowed)
	if err != nil {
		t.Fatalf("NewProperty: %v", err)
	}
	p.SetTempConfig(tc)
	return p
}

func newQuoteUseCase(repo *fakePropertyRepo, blockedRepo *fakeBlockedDatesRepo) *application.QuotePropertyUseCase {
	return application.NewQuotePropertyUseCase(repo, blockedRepo)
}

// ─── Validación de fechas ───────────────────────────────────────────────────

func TestQuote_CheckInVacio_RetornaBadRequest(t *testing.T) {
	uc := newQuoteUseCase(&fakePropertyRepo{}, &fakeBlockedDatesRepo{})

	_, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckOutDate: time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestQuote_CheckOutVacio_RetornaBadRequest(t *testing.T) {
	uc := newQuoteUseCase(&fakePropertyRepo{}, &fakeBlockedDatesRepo{})

	_, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest", err)
	}
}

func TestQuote_CheckOutNoPosteriorACheckIn_RetornaBadRequest(t *testing.T) {
	uc := newQuoteUseCase(&fakePropertyRepo{}, &fakeBlockedDatesRepo{})
	sameDay := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: sameDay, CheckOutDate: sameDay,
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (check_out == check_in)", err)
	}
}

// ─── Propiedad ──────────────────────────────────────────────────────────────

func TestQuote_ErrorBuscandoPropiedad_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, boom },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	_, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5),
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestQuote_PropiedadNoExiste_RetornaNotFound(t *testing.T) {
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return nil, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	_, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "no-existe", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5),
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

// ─── Restricciones de noches ────────────────────────────────────────────────

func TestQuote_MenosDeLasNochesMinimas_RetornaBadRequest(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 3, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	// Solo 2 noches, mínimo son 3
	_, execErr := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 3),
	})

	var appErr *apperr.AppError
	if !errors.As(execErr, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (estadía mínima)", execErr)
	}
}

func TestQuote_MasDeLasNochesMaximas_RetornaBadRequest(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 5, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	// 10 noches, máximo son 5
	_, execErr := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 11),
	})

	var appErr *apperr.AppError
	if !errors.As(execErr, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (estadía máxima)", execErr)
	}
}

// ─── Disponibilidad ─────────────────────────────────────────────────────────

func TestQuote_ErrorVerificandoDisponibilidad_RetornaErrorSinEnvolver(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	boom := errors.New("timeout de base de datos")
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{overlapErr: boom})

	_, execErr := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5),
	})

	if !errors.Is(execErr, boom) {
		t.Fatalf("Execute: got %v, want %v", execErr, boom)
	}
}

func TestQuote_FechasSuperpuestas_RetornaPreconditionFailed(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{hasOverlap: true})

	_, execErr := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5),
	})

	var appErr *apperr.AppError
	if !errors.As(execErr, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("Execute: got %v, want AppError PreconditionFailed", execErr)
	}
}

func TestQuote_ConsultaDisponibilidad_ConParametrosCorrectos(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	blockedRepo := &fakeBlockedDatesRepo{}
	uc := newQuoteUseCase(repo, blockedRepo)
	checkIn, checkOut := date(2025, 1, 1), date(2025, 1, 5)

	if _, err := uc.Execute(context.Background(), application.QuoteCommand{PropertyID: "prop-1", CheckInDate: checkIn, CheckOutDate: checkOut}); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if blockedRepo.calledPropID != "prop-1" || !blockedRepo.calledStart.Equal(checkIn) || !blockedRepo.calledEnd.Equal(checkOut) {
		t.Errorf("HasOverlap args: got propID=%q start=%v end=%v", blockedRepo.calledPropID, blockedRepo.calledStart, blockedRepo.calledEnd)
	}
}

// ─── Cálculo de la cotización ───────────────────────────────────────────────

func TestQuote_HappyPath_SinDescuentoSinFeeSinDeposito(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "15:00", "11:00", 1, 30, 50, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	resp, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5), // 4 noches
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.Nights != 4 || resp.NightPrice != 50 || resp.Subtotal != 200 || resp.DiscountPct != 0 ||
		resp.DiscountAmount != 0 || resp.CleaningFee != 0 || resp.SecurityDeposit != 0 || resp.Total != 200 {
		t.Errorf("QuoteResponse: got %+v", resp)
	}
	if resp.CheckInDate != "2025-01-01" || resp.CheckOutDate != "2025-01-05" || resp.CheckInTime != "15:00" || resp.CheckOutTime != "11:00" {
		t.Errorf("QuoteResponse fechas/horarios: got %+v", resp)
	}
	if len(resp.Breakdown) != 1 {
		t.Fatalf("Breakdown: got %d items, want 1 (solo el subtotal)", len(resp.Breakdown))
	}
}

func TestQuote_HappyPath_ConDescuentoAplicado(t *testing.T) {
	rules := []domain.PricingRule{
		{Type: domain.PricingRuleWeekly, MinNights: 7, DiscountPct: 10},
	}
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 0, 0, rules)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	// 7 noches -> activa el descuento semanal del 10%
	resp, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 8),
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	// Subtotal base = 50*7 = 350; descuento 10% = 35; subtotal final = 315; total = 315 (sin fee)
	if resp.Subtotal != 350 || resp.DiscountPct != 10 || resp.DiscountAmount != 35 || resp.Total != 315 {
		t.Errorf("QuoteResponse cálculo: got %+v", resp)
	}
	if len(resp.Breakdown) != 2 {
		t.Fatalf("Breakdown: got %d items, want 2 (subtotal + descuento)", len(resp.Breakdown))
	}
	if resp.Breakdown[1].Amount != -35 {
		t.Errorf("Breakdown[1] (descuento): got %+v, want Amount=-35", resp.Breakdown[1])
	}
}

func TestQuote_HappyPath_ConCleaningFeeYDeposito(t *testing.T) {
	tc, err := domain.NewTempConfig(nil, "", "", 1, 30, 50, 20, 100, nil)
	if err != nil {
		t.Fatalf("NewTempConfig: %v", err)
	}
	prop := buildTempProperty(t, "prop-1", tc)
	repo := &fakePropertyRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Property, error) { return prop, nil },
	}
	uc := newQuoteUseCase(repo, &fakeBlockedDatesRepo{})

	resp, err := uc.Execute(context.Background(), application.QuoteCommand{
		PropertyID: "prop-1", CheckInDate: date(2025, 1, 1), CheckOutDate: date(2025, 1, 5), // 4 noches
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	// Subtotal = 200, + cleaning fee 20 = Total 220. El depósito NO suma al total (es reintegrable).
	if resp.Subtotal != 200 || resp.CleaningFee != 20 || resp.SecurityDeposit != 100 || resp.Total != 220 {
		t.Errorf("QuoteResponse: got %+v, want Total=220 (depósito excluido del total)", resp)
	}
	if len(resp.Breakdown) != 3 {
		t.Fatalf("Breakdown: got %d items, want 3 (subtotal + limpieza + depósito)", len(resp.Breakdown))
	}
	if resp.Breakdown[1].Description != "Tarifa de limpieza" || resp.Breakdown[1].Amount != 20 {
		t.Errorf("Breakdown[1]: got %+v", resp.Breakdown[1])
	}
	if resp.Breakdown[2].Amount != 100 {
		t.Errorf("Breakdown[2] (depósito): got %+v", resp.Breakdown[2])
	}
}

// date es un helper trivial para no repetir time.Date(...) con hora cero en cada test.
func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
