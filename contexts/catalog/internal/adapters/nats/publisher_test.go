package nats_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	catalognats "inmo.platform/contexts/catalog/internal/adapters/nats"
)

// Nota: este archivo se llama publisher.go pero en realidad implementa un
// *suscriptor* de JetStream (ContractSubscriber) que reacciona a contratos
// firmados dando de baja la propiedad correspondiente — no hay ningún método
// de publicación real acá. Los tests cubren lo que el archivo hace de verdad.

// ─── processMessage (vía StartConsume con un server embebido) ─────────────
//
// processMessage es un método no exportado, así que se ejercita indirectamente
// publicando un mensaje real a un servidor NATS embebido con JetStream y
// dejando correr StartConsume — igual que se probaría en producción.

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// newEmbeddedJetStream levanta un nats-server en memoria con JetStream habilitado
// y devuelve un cliente conectado + su contexto JetStream, listo para crear streams.
func newEmbeddedJetStream(t *testing.T) (*natsgo.Conn, jetstream.JetStream) {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1, // puerto aleatorio libre
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
	return nc, js
}

// newContractsStream crea el stream "contracts" que StartConsume espera encontrar.
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

func TestStartConsume_MensajeValido_ActualizaLaPropiedadYHaceAck(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE properties SET state = 'RENTED'`).
		WithArgs("prop-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	sub := catalognats.NewContractSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{"id": "contract-1", "property_id": "prop-1"})
	if _, err := js.Publish(context.Background(), "contracts.contract.activated", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitForExpectations(t, mock, 3*time.Second)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartConsume: got %v tras cancelar el contexto, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartConsume no retornó tras cancelar el contexto")
	}
}

func TestStartConsume_PropiedadInexistente_NoRompeElConsumidor(t *testing.T) {
	// rows == 0 solo dispara un log de warning — StartConsume debe seguir vivo
	// y acusar recibo del mensaje igual (no hay nada más que reintentar).
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE properties SET state = 'RENTED'`).
		WithArgs("prop-fantasma").
		WillReturnResult(sqlmock.NewResult(0, 0))

	sub := catalognats.NewContractSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{"id": "contract-2", "property_id": "prop-fantasma"})
	if _, err := js.Publish(context.Background(), "contracts.contract.activated", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitForExpectations(t, mock, 3*time.Second)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartConsume: got %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartConsume no retornó tras cancelar el contexto")
	}
}

func TestStartConsume_JSONMalformado_LoggeaYContinuaSinTocarLaDB(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	// Ninguna expectativa de Exec: si processMessage llegara a tocar la DB con
	// un payload inválido, mock.ExpectationsWereMet() lo delataría.

	sub := catalognats.NewContractSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	if _, err := js.Publish(context.Background(), "contracts.contract.activated", []byte("esto no es json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Le damos tiempo al consumidor para procesar (y descartar) el mensaje malformado.
	time.Sleep(500 * time.Millisecond)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartConsume: got %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartConsume no retornó tras cancelar el contexto")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("no debería haberse tocado la DB con un payload inválido: %v", err)
	}
}

func TestStartConsume_ContextoYaCancelado_RetornaErrorEnvuelto(t *testing.T) {
	// Asimetría de comportamiento: la cancelación se maneja "silenciosamente" (nil)
	// solo DESPUÉS de que el consumidor ya se creó, dentro del loop de iter.Next()
	// (rama "if ctx.Err() != nil { return nil }"). Si el contexto ya viene cancelado
	// ANTES de arrancar, CreateOrUpdateConsumer directamente falla con el contexto
	// cancelado y ese error se propaga envuelto — no hay manejo especial para este caso.
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, _ := newMockDB(t)
	sub := catalognats.NewContractSubscriber(db, js)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelado ANTES de arrancar

	err := sub.StartConsume(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StartConsume: got %v, want un error que envuelva context.Canceled", err)
	}
}

func TestStartConsume_StreamInexistente_RetornaErrorEnvuelto(t *testing.T) {
	// Sin crear el stream "contracts", CreateOrUpdateConsumer debe fallar.
	_, js := newEmbeddedJetStream(t)

	db, _ := newMockDB(t)
	sub := catalognats.NewContractSubscriber(db, js)

	err := sub.StartConsume(context.Background())
	if err == nil {
		t.Fatal("StartConsume: esperaba un error al no existir el stream 'contracts'")
	}
}

// waitForExpectations sondea hasta que sqlmock confirme que se cumplieron las
// expectativas configuradas, o falla el test si se agota el timeout. Es necesario
// porque el consumo del mensaje ocurre en una goroutine en background.
func waitForExpectations(t *testing.T, mock sqlmock.Sqlmock, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if lastErr = mock.ExpectationsWereMet(); lastErr == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expectations no cumplidas dentro de %v: %v", timeout, lastErr)
}
