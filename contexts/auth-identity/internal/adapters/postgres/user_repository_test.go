package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/auth-identity/internal/adapters/postgres"
	"inmo.platform/contexts/auth-identity/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// PostgresUserRepository solo se comunica con la base a través de *sql.DB, así
// que sqlmock nos permite verificar el SQL emitido y el mapeo de resultados sin
// levantar un Postgres real. Las queries usan placeholders "$1, $2, ..." propios
// de lib/pq, que son caracteres especiales de regex — por eso todo patrón pasa
// por regexp.QuoteMeta antes de dárselo a sqlmock (que matchea por substring).

func newMockRepo(t *testing.T) (*postgres.PostgresUserRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := postgres.NewPostgresUserRepository(&postgres.DB{Pool: db})
	return repo, mock
}

func assertExpectationsMet(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

var userColumns = []string{"id", "email", "status", "phone", "phone_verified_at", "created_at"}

// ─── Save ───────────────────────────────────────────────────────────────────

func TestSave_HappyPath_ConRolesExplicitos(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}
	roles := []string{"PROPIETARIO", "AGENTE"}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).
		WithArgs(user.ID(), user.Email(), string(user.Status()), user.Phone(), nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).
		WithArgs(provider.ID(), user.ID(), string(provider.Name()), provider.ProviderUserID(), provider.PasswordHash()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	for _, role := range roles {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_roles")).
			WithArgs(user.ID(), role).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	if err := repo.Save(context.Background(), user, provider, roles); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_RolesVacios_UsaINQUILINOPorDefecto(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, err := domain.NewUser("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	provider, err := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	if err != nil {
		t.Fatalf("NewEmailProvider: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_roles")).
		WithArgs(user.ID(), "INQUILINO").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.Save(context.Background(), user, provider, nil); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ProviderSSO_PasswordHashNulo(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, err := domain.NewUserFromSSO("user-1", "user@test.com")
	if err != nil {
		t.Fatalf("NewUserFromSSO: %v", err)
	}
	provider, err := domain.NewSSOProvider("prov-1", "user-1", domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnResult(sqlmock.NewResult(0, 1))
	// A diferencia del provider EMAIL, acá el 5to argumento (password_hash) debe viajar como NULL.
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).
		WithArgs(provider.ID(), user.ID(), string(provider.Name()), provider.ProviderUserID(), nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_roles")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.Save(context.Background(), user, provider, []string{"INTERESADO"}); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ErrorAlIniciarTransaccion_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, _ := domain.NewUser("user-1", "user@test.com")
	provider, _ := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	boom := errors.New("no se pudo abrir la conexión")

	mock.ExpectBegin().WillReturnError(boom)

	err := repo.Save(context.Background(), user, provider, []string{"INQUILINO"})

	if !errors.Is(err, boom) {
		t.Fatalf("Save: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ErrorInsertandoUsuario_HaceRollback(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, _ := domain.NewUser("user-1", "user@test.com")
	provider, _ := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	boom := errors.New("violación de constraint única en email")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Save(context.Background(), user, provider, []string{"INQUILINO"})

	if !errors.Is(err, boom) {
		t.Fatalf("Save: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ErrorInsertandoProvider_HaceRollback(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, _ := domain.NewUser("user-1", "user@test.com")
	provider, _ := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	boom := errors.New("fallo de escritura en identity_providers")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Save(context.Background(), user, provider, []string{"INQUILINO"})

	if !errors.Is(err, boom) {
		t.Fatalf("Save: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ErrorInsertandoRol_HaceRollback(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, _ := domain.NewUser("user-1", "user@test.com")
	provider, _ := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	boom := errors.New("fallo de escritura en user_roles")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_roles")).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Save(context.Background(), user, provider, []string{"PROPIETARIO"})

	if !errors.Is(err, boom) {
		t.Fatalf("Save: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

func TestSave_ErrorEnCommit_SePropagaSinEnvolver(t *testing.T) {
	repo, mock := newMockRepo(t)
	user, _ := domain.NewUser("user-1", "user@test.com")
	provider, _ := domain.NewEmailProvider("prov-1", "user-1", "user@test.com", "Password1")
	boom := errors.New("fallo al confirmar la transacción")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO user_roles")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(boom)

	err := repo.Save(context.Background(), user, provider, []string{"INQUILINO"})

	// El código hace "return tx.Commit()" directo, sin fmt.Errorf — se espera el mismo error.
	if !errors.Is(err, boom) {
		t.Fatalf("Save: got %v, want %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── Update ─────────────────────────────────────────────────────────────────

func TestUpdate_HappyPath(t *testing.T) {
	repo, mock := newMockRepo(t)
	verifiedAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusActive), "+541112345678", &verifiedAt, time.Now())

	mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET status")).
		WithArgs(string(domain.StatusActive), "+541112345678", verifiedAt, "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), user); err != nil {
		t.Fatalf("Update: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestUpdate_Error_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	user := domain.ReconstructUser("user-1", "user@test.com", string(domain.StatusActive), "", nil, time.Now())
	boom := errors.New("fallo de escritura")

	mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET status")).WillReturnError(boom)

	err := repo.Update(context.Background(), user)

	if !errors.Is(err, boom) {
		t.Fatalf("Update: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── FindByID ───────────────────────────────────────────────────────────────

func TestFindByID_HappyPath_ConTelefonoVerificado(t *testing.T) {
	repo, mock := newMockRepo(t)
	verifiedAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	createdAt := time.Date(2023, 6, 15, 8, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE id")).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(userColumns).AddRow("user-1", "user@test.com", "ACTIVE", "+541112345678", verifiedAt, createdAt))

	user, err := repo.FindByID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if user.ID() != "user-1" || user.Email() != "user@test.com" || user.Status() != domain.StatusActive || user.Phone() != "+541112345678" {
		t.Errorf("FindByID: got %+v, want los valores de la fila", user)
	}
	if user.PhoneVerifiedAt() == nil || !user.PhoneVerifiedAt().Equal(verifiedAt) {
		t.Errorf("FindByID PhoneVerifiedAt: got %v, want %v", user.PhoneVerifiedAt(), verifiedAt)
	}
	if !user.CreatedAt().Equal(createdAt) {
		t.Errorf("FindByID CreatedAt: got %v, want %v", user.CreatedAt(), createdAt)
	}
	assertExpectationsMet(t, mock)
}

func TestFindByID_HappyPath_SinTelefonoVerificado(t *testing.T) {
	repo, mock := newMockRepo(t)
	createdAt := time.Date(2023, 6, 15, 8, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE id")).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(userColumns).AddRow("user-1", "user@test.com", "PENDING_VERIFICATION", "", nil, createdAt))

	user, err := repo.FindByID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if user.PhoneVerifiedAt() != nil {
		t.Errorf("FindByID PhoneVerifiedAt: got %v, want nil", user.PhoneVerifiedAt())
	}
	assertExpectationsMet(t, mock)
}

func TestFindByID_NoExiste_RetornaNilSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE id")).
		WithArgs("no-existe").
		WillReturnError(sql.ErrNoRows)

	user, err := repo.FindByID(context.Background(), "no-existe")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if user != nil {
		t.Errorf("FindByID: got %+v, want nil", user)
	}
	assertExpectationsMet(t, mock)
}

func TestFindByID_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE id")).
		WithArgs("user-1").
		WillReturnError(boom)

	_, err := repo.FindByID(context.Background(), "user-1")

	if !errors.Is(err, boom) {
		t.Fatalf("FindByID: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── FindByEmail ────────────────────────────────────────────────────────────

func TestFindByEmail_HappyPath(t *testing.T) {
	repo, mock := newMockRepo(t)
	createdAt := time.Date(2023, 6, 15, 8, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE email")).
		WithArgs("user@test.com").
		WillReturnRows(sqlmock.NewRows(userColumns).AddRow("user-1", "user@test.com", "ACTIVE", "", nil, createdAt))

	user, err := repo.FindByEmail(context.Background(), "user@test.com")

	if err != nil {
		t.Fatalf("FindByEmail: error inesperado: %v", err)
	}
	if user.Email() != "user@test.com" {
		t.Errorf("FindByEmail: got %q, want %q", user.Email(), "user@test.com")
	}
	assertExpectationsMet(t, mock)
}

func TestFindByEmail_NoExiste_RetornaNilSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE email")).
		WithArgs("nadie@test.com").
		WillReturnError(sql.ErrNoRows)

	user, err := repo.FindByEmail(context.Background(), "nadie@test.com")

	if err != nil {
		t.Fatalf("FindByEmail: error inesperado: %v", err)
	}
	if user != nil {
		t.Errorf("FindByEmail: got %+v, want nil", user)
	}
	assertExpectationsMet(t, mock)
}

func TestFindByEmail_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, email, status, phone, phone_verified_at, created_at FROM users WHERE email")).
		WithArgs("user@test.com").
		WillReturnError(boom)

	_, err := repo.FindByEmail(context.Background(), "user@test.com")

	if !errors.Is(err, boom) {
		t.Fatalf("FindByEmail: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── FindRolesByUserID ──────────────────────────────────────────────────────

func TestFindRolesByUserID_HappyPath_MultiplesRoles(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT role FROM user_roles")).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("PROPIETARIO").AddRow("AGENTE"))

	roles, err := repo.FindRolesByUserID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("FindRolesByUserID: error inesperado: %v", err)
	}
	if len(roles) != 2 || roles[0] != "PROPIETARIO" || roles[1] != "AGENTE" {
		t.Errorf("FindRolesByUserID: got %v, want [PROPIETARIO AGENTE]", roles)
	}
	assertExpectationsMet(t, mock)
}

func TestFindRolesByUserID_SinRoles_RetornaVacioSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT role FROM user_roles")).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}))

	roles, err := repo.FindRolesByUserID(context.Background(), "user-1")

	if err != nil {
		t.Fatalf("FindRolesByUserID: error inesperado: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("FindRolesByUserID: got %v, want vacío", roles)
	}
	assertExpectationsMet(t, mock)
}

func TestFindRolesByUserID_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT role FROM user_roles")).
		WithArgs("user-1").
		WillReturnError(boom)

	_, err := repo.FindRolesByUserID(context.Background(), "user-1")

	if !errors.Is(err, boom) {
		t.Fatalf("FindRolesByUserID: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

func TestFindRolesByUserID_ErrorDeScan_SePropagaSinEnvolver(t *testing.T) {
	repo, mock := newMockRepo(t)

	// Un NULL en una columna no nullable (string) hace fallar el Scan a mitad de loop.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT role FROM user_roles")).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(nil))

	_, err := repo.FindRolesByUserID(context.Background(), "user-1")

	if err == nil {
		t.Fatal("FindRolesByUserID: esperaba error de scan")
	}
	assertExpectationsMet(t, mock)
}

// ─── FindProvider ───────────────────────────────────────────────────────────

func TestFindProvider_HappyPath_ConPasswordHash(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, provider_user_id, password_hash FROM identity_providers")).
		WithArgs("user-1", "EMAIL").
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider_user_id", "password_hash"}).AddRow("prov-1", "user@test.com", "hashed-pwd"))

	provider, err := repo.FindProvider(context.Background(), "user-1", domain.ProviderEmail)

	if err != nil {
		t.Fatalf("FindProvider: error inesperado: %v", err)
	}
	if provider.ID() != "prov-1" || provider.ProviderUserID() != "user@test.com" || provider.PasswordHash() != "hashed-pwd" {
		t.Errorf("FindProvider: got %+v", provider)
	}
	assertExpectationsMet(t, mock)
}

func TestFindProvider_HappyPath_SinPasswordHash(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, provider_user_id, password_hash FROM identity_providers")).
		WithArgs("user-1", "GOOGLE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider_user_id", "password_hash"}).AddRow("prov-1", "google-sub-1", nil))

	provider, err := repo.FindProvider(context.Background(), "user-1", domain.ProviderGoogle)

	if err != nil {
		t.Fatalf("FindProvider: error inesperado: %v", err)
	}
	if provider.PasswordHash() != "" {
		t.Errorf("FindProvider PasswordHash: got %q, want vacío", provider.PasswordHash())
	}
	assertExpectationsMet(t, mock)
}

func TestFindProvider_NoExiste_RetornaNilSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, provider_user_id, password_hash FROM identity_providers")).
		WithArgs("user-1", "GOOGLE").
		WillReturnError(sql.ErrNoRows)

	provider, err := repo.FindProvider(context.Background(), "user-1", domain.ProviderGoogle)

	if err != nil {
		t.Fatalf("FindProvider: error inesperado: %v", err)
	}
	if provider != nil {
		t.Errorf("FindProvider: got %+v, want nil", provider)
	}
	assertExpectationsMet(t, mock)
}

func TestFindProvider_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, provider_user_id, password_hash FROM identity_providers")).
		WithArgs("user-1", "EMAIL").
		WillReturnError(boom)

	_, err := repo.FindProvider(context.Background(), "user-1", domain.ProviderEmail)

	if !errors.Is(err, boom) {
		t.Fatalf("FindProvider: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── FindByProviderKey ──────────────────────────────────────────────────────

func TestFindByProviderKey_HappyPath(t *testing.T) {
	repo, mock := newMockRepo(t)
	createdAt := time.Date(2023, 6, 15, 8, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("FROM users u")).
		WithArgs("GOOGLE", "google-sub-1").
		WillReturnRows(sqlmock.NewRows(userColumns).AddRow("user-1", "user@test.com", "ACTIVE", "", nil, createdAt))

	user, err := repo.FindByProviderKey(context.Background(), domain.ProviderGoogle, "google-sub-1")

	if err != nil {
		t.Fatalf("FindByProviderKey: error inesperado: %v", err)
	}
	if user.ID() != "user-1" {
		t.Errorf("FindByProviderKey: got %+v", user)
	}
	assertExpectationsMet(t, mock)
}

func TestFindByProviderKey_NoExiste_RetornaNilSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("FROM users u")).
		WithArgs("GOOGLE", "no-existe").
		WillReturnError(sql.ErrNoRows)

	user, err := repo.FindByProviderKey(context.Background(), domain.ProviderGoogle, "no-existe")

	if err != nil {
		t.Fatalf("FindByProviderKey: error inesperado: %v", err)
	}
	if user != nil {
		t.Errorf("FindByProviderKey: got %+v, want nil", user)
	}
	assertExpectationsMet(t, mock)
}

func TestFindByProviderKey_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("FROM users u")).
		WithArgs("GOOGLE", "google-sub-1").
		WillReturnError(boom)

	_, err := repo.FindByProviderKey(context.Background(), domain.ProviderGoogle, "google-sub-1")

	if !errors.Is(err, boom) {
		t.Fatalf("FindByProviderKey: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── AddProvider ────────────────────────────────────────────────────────────

func TestAddProvider_HappyPath_SiemprePersistePasswordHashNulo(t *testing.T) {
	repo, mock := newMockRepo(t)
	provider, err := domain.NewSSOProvider("prov-1", "user-1", domain.ProviderGoogle, "google-sub-1")
	if err != nil {
		t.Fatalf("NewSSOProvider: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).
		WithArgs(provider.ID(), "user-1", string(provider.Name()), provider.ProviderUserID(), nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.AddProvider(context.Background(), "user-1", provider); err != nil {
		t.Fatalf("AddProvider: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestAddProvider_Error_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	provider, _ := domain.NewSSOProvider("prov-1", "user-1", domain.ProviderGoogle, "google-sub-1")
	boom := errors.New("violación de constraint única")

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO identity_providers")).WillReturnError(boom)

	err := repo.AddProvider(context.Background(), "user-1", provider)

	if !errors.Is(err, boom) {
		t.Fatalf("AddProvider: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── SaveVerificationToken ──────────────────────────────────────────────────

func TestSaveVerificationToken_HappyPath(t *testing.T) {
	repo, mock := newMockRepo(t)
	token := domain.NewEmailVerificationToken("tok-1", "user-1")

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO verification_tokens")).
		WithArgs(token.Value(), string(token.Type()), token.UserID(), sqlmock.AnyArg(), nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.SaveVerificationToken(context.Background(), token); err != nil {
		t.Fatalf("SaveVerificationToken: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestSaveVerificationToken_Error_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	token := domain.NewEmailVerificationToken("tok-1", "user-1")
	boom := errors.New("fallo de escritura")

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO verification_tokens")).WillReturnError(boom)

	err := repo.SaveVerificationToken(context.Background(), token)

	if !errors.Is(err, boom) {
		t.Fatalf("SaveVerificationToken: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── FindVerificationToken ──────────────────────────────────────────────────

var tokenColumns = []string{"token", "token_type", "user_id", "expires_at", "used_at"}

func TestFindVerificationToken_HappyPath_SinUsar(t *testing.T) {
	repo, mock := newMockRepo(t)
	expiresAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT token, token_type, user_id, expires_at, used_at FROM verification_tokens")).
		WithArgs("tok-1", "EMAIL_VERIFICATION").
		WillReturnRows(sqlmock.NewRows(tokenColumns).AddRow("tok-1", "EMAIL_VERIFICATION", "user-1", expiresAt, nil))

	token, err := repo.FindVerificationToken(context.Background(), "tok-1", domain.TypeEmailVerification)

	if err != nil {
		t.Fatalf("FindVerificationToken: error inesperado: %v", err)
	}
	if token.Value() != "tok-1" || token.UserID() != "user-1" || token.Type() != domain.TypeEmailVerification {
		t.Errorf("FindVerificationToken: got %+v", token)
	}
	if token.UsedAt() != nil {
		t.Errorf("FindVerificationToken UsedAt: got %v, want nil", token.UsedAt())
	}
	assertExpectationsMet(t, mock)
}

func TestFindVerificationToken_HappyPath_YaUsado(t *testing.T) {
	repo, mock := newMockRepo(t)
	expiresAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	usedAt := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT token, token_type, user_id, expires_at, used_at FROM verification_tokens")).
		WithArgs("tok-1", "EMAIL_VERIFICATION").
		WillReturnRows(sqlmock.NewRows(tokenColumns).AddRow("tok-1", "EMAIL_VERIFICATION", "user-1", expiresAt, usedAt))

	token, err := repo.FindVerificationToken(context.Background(), "tok-1", domain.TypeEmailVerification)

	if err != nil {
		t.Fatalf("FindVerificationToken: error inesperado: %v", err)
	}
	if token.UsedAt() == nil || !token.UsedAt().Equal(usedAt) {
		t.Errorf("FindVerificationToken UsedAt: got %v, want %v", token.UsedAt(), usedAt)
	}
	assertExpectationsMet(t, mock)
}

func TestFindVerificationToken_NoExiste_RetornaNilSinError(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT token, token_type, user_id, expires_at, used_at FROM verification_tokens")).
		WithArgs("no-existe", "EMAIL_VERIFICATION").
		WillReturnError(sql.ErrNoRows)

	token, err := repo.FindVerificationToken(context.Background(), "no-existe", domain.TypeEmailVerification)

	if err != nil {
		t.Fatalf("FindVerificationToken: error inesperado: %v", err)
	}
	if token != nil {
		t.Errorf("FindVerificationToken: got %+v, want nil", token)
	}
	assertExpectationsMet(t, mock)
}

func TestFindVerificationToken_ErrorDeQuery_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	boom := errors.New("timeout de conexión")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT token, token_type, user_id, expires_at, used_at FROM verification_tokens")).
		WithArgs("tok-1", "PHONE_OTP").
		WillReturnError(boom)

	_, err := repo.FindVerificationToken(context.Background(), "tok-1", domain.TypePhoneOTP)

	if !errors.Is(err, boom) {
		t.Fatalf("FindVerificationToken: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}

// ─── UpdateVerificationToken ────────────────────────────────────────────────

func TestUpdateVerificationToken_HappyPath(t *testing.T) {
	repo, mock := newMockRepo(t)
	token := domain.NewEmailVerificationToken("tok-1", "user-1")
	if err := token.Use(); err != nil {
		t.Fatalf("token.Use: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE verification_tokens SET used_at")).
		WithArgs(sqlmock.AnyArg(), token.Value()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateVerificationToken(context.Background(), token); err != nil {
		t.Fatalf("UpdateVerificationToken: error inesperado: %v", err)
	}
	assertExpectationsMet(t, mock)
}

func TestUpdateVerificationToken_Error_RetornaErrorEnvuelto(t *testing.T) {
	repo, mock := newMockRepo(t)
	token := domain.NewEmailVerificationToken("tok-1", "user-1")
	boom := errors.New("fallo de escritura")

	mock.ExpectExec(regexp.QuoteMeta("UPDATE verification_tokens SET used_at")).WillReturnError(boom)

	err := repo.UpdateVerificationToken(context.Background(), token)

	if !errors.Is(err, boom) {
		t.Fatalf("UpdateVerificationToken: got %v, want error que envuelva %v", err, boom)
	}
	assertExpectationsMet(t, mock)
}
