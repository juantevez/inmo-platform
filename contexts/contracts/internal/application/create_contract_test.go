package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func validCreateContractDTO() application.CreateContractDTO {
	return application.CreateContractDTO{
		ID:               "contract-1",
		PropertyID:       "prop-1",
		TenantID:         "tenant-1",
		OwnerID:          "owner-1",
		Amount:           100000,
		Currency:         "ARS",
		StartDate:        time.Now(),
		EndDate:          time.Now().Add(365 * 24 * time.Hour),
		AdjustmentIndex:  "ICL",
		AdjustmentPeriod: 4,
	}
}

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestCreateContractUseCase_MontoInvalido_RetornaBadRequest(t *testing.T) {
	repo := &fakeContractRepo{}
	uc := application.NewCreateContractUseCase(repo)

	dto := validCreateContractDTO()
	dto.Amount = 0

	err := uc.Execute(context.Background(), dto)
	assertBadRequest(t, err)
}

func TestCreateContractUseCase_MonedaVacia_RetornaBadRequest(t *testing.T) {
	repo := &fakeContractRepo{}
	uc := application.NewCreateContractUseCase(repo)

	dto := validCreateContractDTO()
	dto.Currency = ""

	err := uc.Execute(context.Background(), dto)
	assertBadRequest(t, err)
}

func TestCreateContractUseCase_FechasInvalidas_RetornaBadRequest(t *testing.T) {
	repo := &fakeContractRepo{}
	uc := application.NewCreateContractUseCase(repo)

	dto := validCreateContractDTO()
	dto.EndDate = dto.StartDate.Add(-24 * time.Hour) // end antes de start

	err := uc.Execute(context.Background(), dto)
	assertBadRequest(t, err)
}

func TestCreateContractUseCase_PeriodoDeAjusteNegativo_RetornaBadRequest(t *testing.T) {
	repo := &fakeContractRepo{}
	uc := application.NewCreateContractUseCase(repo)

	dto := validCreateContractDTO()
	dto.AdjustmentPeriod = -1

	err := uc.Execute(context.Background(), dto)
	assertBadRequest(t, err)
}

func TestCreateContractUseCase_ErrorAlGuardar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("fallo de conexión")
	repo := &fakeContractRepo{
		saveFn: func(ctx context.Context, c *domain.Contract) error { return dbErr },
	}
	uc := application.NewCreateContractUseCase(repo)

	err := uc.Execute(context.Background(), validCreateContractDTO())
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestCreateContractUseCase_Exitoso_ConstruyeYGuardaElContratoEnDraft(t *testing.T) {
	var saved *domain.Contract
	repo := &fakeContractRepo{
		saveFn: func(ctx context.Context, c *domain.Contract) error {
			saved = c
			return nil
		},
	}
	uc := application.NewCreateContractUseCase(repo)

	dto := validCreateContractDTO()
	if err := uc.Execute(context.Background(), dto); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if saved == nil {
		t.Fatal("esperaba que se llame a Save")
	}
	if saved.ID() != dto.ID || saved.PropertyID() != dto.PropertyID || saved.TenantID() != dto.TenantID || saved.OwnerID() != dto.OwnerID {
		t.Fatalf("identidad mapeada incorrectamente: %+v", saved)
	}
	if saved.State() != domain.StateDraft {
		t.Fatalf("state: got %s, want %s", saved.State(), domain.StateDraft)
	}
	if saved.RentAmount().Amount() != dto.Amount || saved.RentAmount().Currency() != dto.Currency {
		t.Fatalf("rent amount mapeado incorrectamente: %+v", saved.RentAmount())
	}
	if saved.Index() != domain.AdjustmentIndex(dto.AdjustmentIndex) || saved.AdjustmentPeriod() != domain.AdjustmentPeriod(dto.AdjustmentPeriod) {
		t.Fatalf("index/period mapeados incorrectamente: %s / %d", saved.Index(), saved.AdjustmentPeriod())
	}
	if len(saved.PullEvents()) != 0 {
		t.Fatal("un contrato recién creado (sin Activate) no debería tener eventos pendientes")
	}
}
