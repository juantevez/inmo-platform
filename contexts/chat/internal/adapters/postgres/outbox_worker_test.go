package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/chat/internal/adapters/postgres"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// ChatOutboxWorker construye su publisher internamente vía eventbus.NewEventPublisher(js),
// como un *eventbus.EventPublisher concreto (no una interfaz inyectable) — así que
// para ejercitar Publish() de verdad hace falta un servidor NATS/JetStream embebido
// real, igual que en los tests de los subscribers.

func newEmbeddedJetStream(t *testing.T) jetstream.JetStream {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
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

func newChatStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "chat",
		Subjects: []string{"chat.>"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
}

// runStartAndCancel arranca Start() en background con un intervalo corto,
// espera a que se cumpla la condición dada (o falla el test), y cancela el
// contexto para confirmar que Start() retorna limpiamente.
func runStartAndCancel(t *testing.T, worker *postgres.ChatOutboxWorker, cond func() bool, waitFor time.Duration) {
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

var outboxColumns = []string{"id", "subject", "payload"}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestChatOutboxWorker_SeDetieneAlCancelarElContexto(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	// Sin filas pendientes en ningún tick — cada ciclo abre/cierra tx sin commitear.
	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).WillReturnRows(sqlmock.NewRows(outboxColumns))
	mock.ExpectRollback()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx, 50*time.Millisecond)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond) // deja pasar al menos un tick
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func TestChatOutboxWorker_ErrorAlIniciarTransaccion_LoggeaYContinua(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin().WillReturnError(errors.New("conexión rechazada"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { worker.Start(ctx, 30*time.Millisecond); close(done) }()

	time.Sleep(80 * time.Millisecond) // el worker no debería crashear
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func TestChatOutboxWorker_ErrorDeQuery_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).WillReturnError(errors.New("timeout de base de datos"))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestChatOutboxWorker_ErrorDeScan_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	// NULL en "id" (columna no nullable en el destino string) rompe el Scan.
	mock.ExpectQuery(`SELECT id, subject, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow(nil, "chat.message.sent", []byte(`{}`)))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestChatOutboxWorker_SinEventosPendientes_NoPublicaYHaceRollback(t *testing.T) {
	// Cuando no hay filas, processEvents retorna nil SIN llamar tx.Commit() —
	// el defer tx.Rollback() sí llega a ejecutarse contra el driver.
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).WillReturnRows(sqlmock.NewRows(outboxColumns))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 2*time.Second)
}

func TestChatOutboxWorker_HappyPath_PublicaYMarcaProcessed(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newChatStream(t, js)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).
			AddRow("evt-1", "chat.message.sent", []byte(`{"foo":"bar"}`)).
			AddRow("evt-2", "chat.visit.proposed", []byte(`{}`)))
	mock.ExpectExec(`UPDATE chat_outbox_events`).WithArgs("evt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE chat_outbox_events`).WithArgs("evt-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
}

func TestChatOutboxWorker_ErrorAlPublicar_HaceRollbackYNoMarcaProcessed(t *testing.T) {
	// Sin ningún stream que cubra "chat.message.sent", js.Publish falla — el
	// worker debe abortar antes de intentar el UPDATE y hacer rollback.
	js := newEmbeddedJetStream(t)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow("evt-1", "chat.message.sent", []byte(`{}`)))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 6*time.Second)
}

func TestChatOutboxWorker_ErrorEnUpdate_HaceRollback(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newChatStream(t, js)
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).AddRow("evt-1", "chat.message.sent", []byte(`{}`)))
	mock.ExpectExec(`UPDATE chat_outbox_events`).WithArgs("evt-1").WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
}

func TestChatOutboxWorker_UnEventoFallaPublicando_RevierteTambienLosYaProcesados(t *testing.T) {
	// El batch entero corre en una única transacción: si el segundo evento
	// falla al publicar, el rollback deshace también el UPDATE del primero
	// (que sí se había publicado con éxito) — se va a reintentar en el próximo tick.
	js := newEmbeddedJetStream(t)
	newChatStream(t, js) // cubre "chat.>", pero el segundo evento usa un subject fuera de eso
	db, mock := newMockDB(t)
	worker := postgres.NewChatOutboxWorker(db, js)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject, payload`).
		WillReturnRows(sqlmock.NewRows(outboxColumns).
			AddRow("evt-1", "chat.message.sent", []byte(`{}`)).
			AddRow("evt-2", "otro.contexto.sin.stream", []byte(`{}`)))
	mock.ExpectExec(`UPDATE chat_outbox_events`).WithArgs("evt-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	runStartAndCancel(t, worker, func() bool { return mock.ExpectationsWereMet() == nil }, 6*time.Second)
}
