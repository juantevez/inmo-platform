package nats_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	catalognats "inmo.platform/contexts/catalog/internal/adapters/nats"
)

// Reutiliza newEmbeddedJetStream, newContractsStream, newMockDB y
// waitForExpectations definidos en publisher_test.go (mismo paquete nats_test).
// ReservationSubscriber comparte el stream "contracts" pero filtra por el
// subject "contracts.reservation.confirmed".

func TestReservationStartConsume_MensajeValido_BloqueaFechasYHaceAck(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO property_blocked_dates`).
		WithArgs(sqlmock.AnyArg(), "prop-1", "2025-01-01", "2025-01-05", "res-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	sub := catalognats.NewReservationSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{
		"reservation_id": "res-1", "property_id": "prop-1",
		"check_in_date": "2025-01-01", "check_out_date": "2025-01-05",
	})
	if _, err := js.Publish(context.Background(), "contracts.reservation.confirmed", payload); err != nil {
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

func TestReservationStartConsume_CheckInInvalido_NoTocaLaDB(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	// Sin expectativas de Exec: una fecha inválida debe frenar antes del INSERT.

	sub := catalognats.NewReservationSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{
		"reservation_id": "res-1", "property_id": "prop-1",
		"check_in_date": "01/01/2025", "check_out_date": "2025-01-05",
	})
	if _, err := js.Publish(context.Background(), "contracts.reservation.confirmed", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

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
		t.Errorf("no debería haberse tocado la DB con check_in_date inválida: %v", err)
	}
}

func TestReservationStartConsume_CheckOutInvalido_NoTocaLaDB(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)

	sub := catalognats.NewReservationSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{
		"reservation_id": "res-1", "property_id": "prop-1",
		"check_in_date": "2025-01-01", "check_out_date": "05/01/2025",
	})
	if _, err := js.Publish(context.Background(), "contracts.reservation.confirmed", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

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
		t.Errorf("no debería haberse tocado la DB con check_out_date inválida: %v", err)
	}
}

func TestReservationStartConsume_JSONMalformado_NoTocaLaDB(t *testing.T) {
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)

	sub := catalognats.NewReservationSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	if _, err := js.Publish(context.Background(), "contracts.reservation.confirmed", []byte("esto no es json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

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

func TestReservationStartConsume_ErrorAlInsertar_NoRompeElConsumidor(t *testing.T) {
	// Un fallo de INSERT (ej: constraint) solo debe loggearse — el consumidor
	// sigue vivo y responde bien a la cancelación del contexto.
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, mock := newMockDB(t)
	mock.ExpectExec(`INSERT INTO property_blocked_dates`).
		WithArgs(sqlmock.AnyArg(), "prop-1", "2025-01-01", "2025-01-05", "res-1").
		WillReturnError(errors.New("constraint violada"))

	sub := catalognats.NewReservationSubscriber(db, js)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(ctx) }()

	payload, _ := json.Marshal(map[string]string{
		"reservation_id": "res-1", "property_id": "prop-1",
		"check_in_date": "2025-01-01", "check_out_date": "2025-01-05",
	})
	if _, err := js.Publish(context.Background(), "contracts.reservation.confirmed", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitForExpectations(t, mock, 3*time.Second)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartConsume: got %v, want nil (el error de insert no debe tumbar el consumidor)", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StartConsume no retornó tras cancelar el contexto")
	}
}

func TestReservationStartConsume_ContextoYaCancelado_RetornaErrorEnvuelto(t *testing.T) {
	// Misma asimetría que ContractSubscriber: la cancelación "silenciosa" (nil)
	// solo aplica DESPUÉS de crear el consumidor, dentro del loop de iter.Next().
	_, js := newEmbeddedJetStream(t)
	newContractsStream(t, js)

	db, _ := newMockDB(t)
	sub := catalognats.NewReservationSubscriber(db, js)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sub.StartConsume(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StartConsume: got %v, want un error que envuelva context.Canceled", err)
	}
}

func TestReservationStartConsume_StreamInexistente_RetornaErrorEnvuelto(t *testing.T) {
	_, js := newEmbeddedJetStream(t)

	db, _ := newMockDB(t)
	sub := catalognats.NewReservationSubscriber(db, js)

	err := sub.StartConsume(context.Background())
	if err == nil {
		t.Fatal("StartConsume: esperaba un error al no existir el stream 'contracts'")
	}
}
