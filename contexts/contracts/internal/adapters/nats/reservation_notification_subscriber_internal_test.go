package nats

// Test de caja blanca (mismo package, no "nats_test"): dos ramas que no son
// alcanzables desde afuera.
//   - El valor por defecto de chatURL es un detalle interno sin getter.
//   - El "default" del switch en processMessage es inalcanzable vía
//     StartConsume real porque el consumer se crea con FilterSubjects
//     restringido a los dos subjects que sí se manejan explícitamente.

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeMsg implementa jetstream.Msg únicamente para poder invocar
// processMessage() directamente; solo Subject()/Data() importan para este test.
type fakeMsg struct {
	subject string
	data    []byte
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) { return nil, nil }
func (m *fakeMsg) Data() []byte                              { return m.data }
func (m *fakeMsg) Headers() nats.Header                      { return nil }
func (m *fakeMsg) Subject() string                           { return m.subject }
func (m *fakeMsg) Reply() string                             { return "" }
func (m *fakeMsg) Ack() error                                { return nil }
func (m *fakeMsg) DoubleAck(ctx context.Context) error       { return nil }
func (m *fakeMsg) Nak() error                                { return nil }
func (m *fakeMsg) NakWithDelay(delay time.Duration) error    { return nil }
func (m *fakeMsg) InProgress() error                         { return nil }
func (m *fakeMsg) Term() error                               { return nil }
func (m *fakeMsg) TermWithReason(reason string) error        { return nil }

func TestNewReservationNotificationSubscriber_SinGatewayURL_UsaElDefault(t *testing.T) {
	t.Setenv("GATEWAY_URL", "")

	sub := NewReservationNotificationSubscriber(nil)

	if sub.chatURL != "http://localhost:8000" {
		t.Fatalf("chatURL: got %q, want %q", sub.chatURL, "http://localhost:8000")
	}
}

func TestNewReservationNotificationSubscriber_ConGatewayURL_LoUsaTalCual(t *testing.T) {
	t.Setenv("GATEWAY_URL", "http://gateway-interno:9000")

	sub := NewReservationNotificationSubscriber(nil)

	if sub.chatURL != "http://gateway-interno:9000" {
		t.Fatalf("chatURL: got %q, want %q", sub.chatURL, "http://gateway-interno:9000")
	}
}

func TestReservationNotificationSubscriber_ProcessMessage_SubjectInesperado_LoIgnora(t *testing.T) {
	// FilterSubjects en StartConsume nunca entregaría esto en producción, pero
	// documentamos el comportamiento defensivo del default del switch.
	sub := NewReservationNotificationSubscriber(nil)

	msg := &fakeMsg{subject: "contracts.reservation.otro-evento", data: []byte(`{}`)}
	if err := sub.processMessage(context.Background(), msg); err != nil {
		t.Fatalf("un subject no manejado explícitamente debería ignorarse sin error: %v", err)
	}
}
