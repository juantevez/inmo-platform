package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/contracts/internal/adapters/postgres"
	"inmo.platform/contexts/contracts/internal/domain"
)

var snapshotColumns = []string{
	"property_id", "owner_id", "operation_type", "night_price", "cleaning_fee", "security_deposit",
	"min_nights", "max_nights", "check_in_time", "check_out_time", "pricing_rules", "updated_at",
}

func validSnapshotFixture() domain.PropertySnapshot {
	return domain.PropertySnapshot{
		PropertyID:      "prop-1",
		OwnerID:         "owner-1",
		OperationType:   "TEMP",
		NightPrice:      15000,
		CleaningFee:     2000,
		SecurityDeposit: 8000,
		MinNights:       2,
		MaxNights:       30,
		CheckInTime:     "14:00",
		CheckOutTime:    "10:00",
		PricingRules:    []domain.PricingRule{{Type: "weekly", MinNights: 7, DiscountPct: 10}},
	}
}

// ─── Upsert ─────────────────────────────────────────────────────────────────

func TestSnapshotRepository_Upsert_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO property_snapshots`).WillReturnError(errors.New("fallo de escritura"))
	repo := postgres.NewSnapshotRepository(db)

	err := repo.Upsert(context.Background(), validSnapshotFixture())
	assertInternalError(t, err)
}

func TestSnapshotRepository_Upsert_Exitoso_MapeaTodosLosCampos(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO property_snapshots`).
		WithArgs("prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := postgres.NewSnapshotRepository(db)

	if err := repo.Upsert(context.Background(), validSnapshotFixture()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSnapshotRepository_Upsert_SinPricingRules_SerializaListaVacia(t *testing.T) {
	db, mock := newMockDB(t)
	snap := validSnapshotFixture()
	snap.PricingRules = nil

	mock.ExpectExec(`INSERT INTO property_snapshots`).
		WithArgs("prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", []byte("null"), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	repo := postgres.NewSnapshotRepository(db)

	if err := repo.Upsert(context.Background(), snap); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// ─── FindByID ───────────────────────────────────────────────────────────────

func TestSnapshotRepository_FindByID_NoEncontrado_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT property_id, owner_id, operation_type,`).
		WithArgs("prop-x").
		WillReturnRows(sqlmock.NewRows(snapshotColumns))
	repo := postgres.NewSnapshotRepository(db)

	snap, err := repo.FindByID(context.Background(), "prop-x")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if snap != nil {
		t.Fatalf("esperaba nil, obtuve %+v", snap)
	}
}

func TestSnapshotRepository_FindByID_ErrorDeDB(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT property_id, owner_id, operation_type,`).
		WithArgs("prop-1").
		WillReturnError(errors.New("timeout de red"))
	repo := postgres.NewSnapshotRepository(db)

	_, err := repo.FindByID(context.Background(), "prop-1")
	assertInternalError(t, err)
}

func TestSnapshotRepository_FindByID_Exitoso_MapeaYDeserializaPricingRules(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rulesJSON := `[{"type":"weekly","min_nights":7,"discount_pct":10},{"type":"monthly","min_nights":30,"discount_pct":20}]`
	mock.ExpectQuery(`SELECT property_id, owner_id, operation_type,`).
		WithArgs("prop-1").
		WillReturnRows(sqlmock.NewRows(snapshotColumns).AddRow(
			"prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", []byte(rulesJSON), now,
		))
	repo := postgres.NewSnapshotRepository(db)

	snap, err := repo.FindByID(context.Background(), "prop-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if snap == nil {
		t.Fatal("esperaba un snapshot, obtuve nil")
	}
	if snap.PropertyID != "prop-1" || snap.OwnerID != "owner-1" || snap.OperationType != "TEMP" {
		t.Fatalf("identidad mapeada incorrectamente: %+v", snap)
	}
	if snap.NightPrice != 15000 || snap.CleaningFee != 2000 || snap.SecurityDeposit != 8000 {
		t.Fatalf("montos mapeados incorrectamente: %+v", snap)
	}
	if snap.MinNights != 2 || snap.MaxNights != 30 || snap.CheckInTime != "14:00" || snap.CheckOutTime != "10:00" {
		t.Fatalf("restricciones/horarios mapeados incorrectamente: %+v", snap)
	}
	if len(snap.PricingRules) != 2 || snap.PricingRules[1].MinNights != 30 || snap.PricingRules[1].DiscountPct != 20 {
		t.Fatalf("pricing_rules deserializadas incorrectamente: %+v", snap.PricingRules)
	}
	if !snap.UpdatedAt.Equal(now) {
		t.Fatalf("updatedAt: got %v, want %v", snap.UpdatedAt, now)
	}
}

func TestSnapshotRepository_FindByID_PricingRulesVacias_QuedaNil(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT property_id, owner_id, operation_type,`).
		WithArgs("prop-1").
		WillReturnRows(sqlmock.NewRows(snapshotColumns).AddRow(
			"prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", []byte{}, now,
		))
	repo := postgres.NewSnapshotRepository(db)

	snap, err := repo.FindByID(context.Background(), "prop-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if snap.PricingRules != nil {
		t.Fatalf("esperaba PricingRules nil, obtuve %+v", snap.PricingRules)
	}
}

func TestSnapshotRepository_FindByID_PricingRulesCorruptas_RetornaErrorInterno(t *testing.T) {
	// Regresión: FindByID debe propagar un error si pricing_rules tiene JSON
	// corrupto, en vez de silenciarlo y devolver el snapshot con la lista vacía.
	db, mock := newMockDB(t)
	now := time.Now()
	mock.ExpectQuery(`SELECT property_id, owner_id, operation_type,`).
		WithArgs("prop-1").
		WillReturnRows(sqlmock.NewRows(snapshotColumns).AddRow(
			"prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", []byte(`{esto no es json valido`), now,
		))
	repo := postgres.NewSnapshotRepository(db)

	_, err := repo.FindByID(context.Background(), "prop-1")
	assertInternalError(t, err)
}
