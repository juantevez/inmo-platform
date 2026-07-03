package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/contracts/internal/adapters/postgres"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// OutboxWorker construye su publisher internamente vía eventbus.NewEventPublisher(js),
// como un *eventbus.EventPublisher concreto (no una interfaz inyectable) — así que
// para ejercitar Publish() de verdad hace falta un servidor NATS/JetStream embebido
// real, igual que en el outbox worker del contexto chat.

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func newEmbeddedJetStream(t *testing.T) jetstream.JetStream {
	t.Helper()

	opts := &natsserver.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir()}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("natsserver.NewServer: %v", err)
	}
	go srv.Start()
	t.Cleanup(srv.Shutdown)

	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("el servidor NATS embebido no arrancó a tiempo")
	}

	nc, err := natsgo.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("nats.Connect: %v", err)
	}
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	return js
}

func newContractsStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "contracts",
		Subjects: []string{"contracts.>"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
}

func runStartAndCancel(t *testing.T, worker *postgres.OutboxWorker, cond func() bool, waitFor time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx, 20*time.Millisecond)
		close(done)
	}()

	waitUntil(t, waitFor, cond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condición no se cumplió dentro del timeout")
}

var outboxColumns = []string{"id", "event_name", "payload"}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestOutboxWorker_SeDetieneAlCancelarElContexto(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).WillReturnRows(sqlmock.NewRows(outboxColumns))
	mock.ExpectRollback()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx, 50*time.Millisecond)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func TestOutboxWorker_ErrorAlIniciarTransaccion_LoggeaYContinua(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { worker.Start(ctx, 30*time.Millisecond); close(done) }()

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func TestOutboxWorker_ErrorDeQuery_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).WillReturnError(errors.New("timeout de base de datos"))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestOutboxWorker_ErrorDeScan_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow(nil, "contracts.contract.activated", []byte(`{}`)))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestOutboxWorker_SinEventosPendientes_NoPublicaYHaceRollback(t *testing.T) {
	// Cuando no hay filas, processEvents retorna nil SIN llamar tx.Commit() —
	// el defer tx.Rollback() sí llega a ejecutarse contra el driver.
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).WillReturnRows(sqlmock.NewRows(outboxColumns))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestOutboxWorker_HappyPath_PublicaYMarcaProcessed(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).
			AddRow("evt-1", "contracts.contract.activated", []byte(`{"foo":"bar"}`)).
			AddRow("evt-2", "contracts.reservation.confirmed", []byte(`{}`)))
	mock.ExpectExec(`UPDATE contracts_outbox_events`).WithArgs("evt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contracts_outbox_events`).WithArgs("evt-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
}

func TestOutboxWorker_ErrorEnUpdate_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newContractsStream(t, js)
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow("evt-1", "contracts.contract.activated", []byte(`{}`)))
	mock.ExpectExec(`UPDATE contracts_outbox_events`).WithArgs("evt-1").WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
}

func TestOutboxWorker_ErrorAlPublicar_SalteaElEventoPeroCommiteaElResto(t *testing.T) {
	// A diferencia del outbox worker de chat (que aborta TODO el lote si un
	// evento falla al publicar), este worker hace `continue` ante un error de
	// Publish: el evento fallido queda PENDING (se reintentará en el próximo
	// tick) pero los eventos ya publicados con éxito EN EL MISMO LOTE sí se
	// commitean — no se revierten. Confirmamos ese comportamiento acá.
	js := newEmbeddedJetStream(t)
	newContractsStream(t, js) // cubre "contracts.>"; el segundo evento usa un subject sin stream
	db, mock := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).
			AddRow("evt-1", "contracts.contract.activated", []byte(`{}`)).
			AddRow("evt-2", "otro.contexto.sin.stream", []byte(`{}`)).
			AddRow("evt-3", "contracts.reservation.confirmed", []byte(`{}`)))
	mock.ExpectExec(`UPDATE contracts_outbox_events`).WithArgs("evt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	// evt-2 falla al publicar: NO se espera ningún UPDATE para su ID.
	mock.ExpectExec(`UPDATE contracts_outbox_events`).WithArgs("evt-3").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 6*time.Second)
}

func TestOutboxWorker_TodosLosEventosFallanAlPublicar_IgualCommitea(t *testing.T) {
	// Si NINGÚN evento del lote logra publicarse, el loop nunca hace ExecContext
	// ni retorna error — igual llega a tx.Commit() (un commit "vacío", sin cambios).
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t) // sin stream "contracts": cualquier Publish falla

	worker := postgres.NewOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_name, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow("evt-1", "contracts.contract.activated", []byte(`{}`)))
	mock.ExpectCommit()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 6*time.Second)
}
