package nats_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	natsadapter "inmo.platform/contexts/contracts/internal/adapters/nats"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// PropertySubscriber construye internamente un *postgres.SnapshotRepository
// concreto (no una interfaz inyectable), así que se necesita un *sql.DB real
// (sqlmock) combinado con un JetStream real embebido, igual que en
// outbox_worker_test.go del contexto chat.

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

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func newCatalogStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "catalog",
		Subjects: []string{"catalog.>"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
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

func publishJSON(t *testing.T, js jetstream.JetStream, subject, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := js.Publish(ctx, subject, []byte(payload)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func runSubscribeAndCancel(t *testing.T, start func(ctx context.Context) error, waitFor func() bool, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- start(ctx) }()

	if waitFor != nil {
		waitUntil(t, timeout, waitFor)
	} else {
		time.Sleep(150 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartConsume debería retornar nil tras cancelar el contexto: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartConsume no retornó tras cancelar el contexto")
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestPropertySubscriber_PropiedadTemporaria_ActualizaElSnapshot(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	db, mock := newMockDB(t)

	mock.ExpectExec(`INSERT INTO property_snapshots`).
		WithArgs("prop-1", "owner-1", "TEMP", 15000.0, 2000.0, 8000.0, 2, 30, "14:00", "10:00", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	sub := natsadapter.NewPropertySubscriber(db, js)

	payload := `{
		"aggregate_id": "prop-1",
		"snapshot": {
			"owner_id": "owner-1",
			"operation_type": "TEMP",
			"night_price": 15000,
			"cleaning_fee": 2000,
			"security_deposit": 8000,
			"min_nights": 2,
			"max_nights": 30,
			"check_in_time": "14:00",
			"check_out_time": "10:00",
			"pricing_rules": [{"type":"weekly","min_nights":7,"discount_pct":10}]
		}
	}`
	publishJSON(t, js, "catalog.property.published", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPropertySubscriber_PropiedadNoTemporaria_IgnoraElEvento(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	db, mock := newMockDB(t)
	// Ninguna expectativa configurada: cualquier llamada a la DB haría fallar el test.

	sub := natsadapter.NewPropertySubscriber(db, js)

	payload := `{
		"aggregate_id": "prop-1",
		"snapshot": {"operation_type": "RENT", "night_price": 0}
	}`
	publishJSON(t, js, "catalog.property.published", payload)

	runSubscribeAndCancel(t, sub.StartConsume, nil, 0)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no debería haberse tocado la DB para una propiedad no-TEMP: %v", err)
	}
}

func TestPropertySubscriber_JSONMalformado_NoTocaLaDBYSigueVivo(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	db, mock := newMockDB(t)

	sub := natsadapter.NewPropertySubscriber(db, js)

	publishJSON(t, js, "catalog.property.published", `{"invalido"`)

	runSubscribeAndCancel(t, sub.StartConsume, nil, 0)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no debería haberse tocado la DB con un JSON malformado: %v", err)
	}
}

func TestPropertySubscriber_ErrorAlGuardarElSnapshot_NoRompeElLoop(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO property_snapshots`).WillReturnError(sql.ErrConnDone)

	sub := natsadapter.NewPropertySubscriber(db, js)

	payload := `{
		"aggregate_id": "prop-1",
		"snapshot": {"operation_type": "TEMP", "night_price": 15000}
	}`
	publishJSON(t, js, "catalog.property.published", payload)

	runSubscribeAndCancel(t, sub.StartConsume, func() bool { return mock.ExpectationsWereMet() == nil }, 3*time.Second)
}

func TestPropertySubscriber_CancelacionDeContexto_TerminaLimpiamente(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	db, _ := newMockDB(t)

	sub := natsadapter.NewPropertySubscriber(db, js)

	runSubscribeAndCancel(t, sub.StartConsume, nil, 0)
}
