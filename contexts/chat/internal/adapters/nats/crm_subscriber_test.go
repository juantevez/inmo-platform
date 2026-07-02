package nats_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	chatnats "inmo.platform/contexts/chat/internal/adapters/nats"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeVisitProposalRepo struct {
	mu         sync.Mutex
	findByIDFn func(ctx context.Context, id string) (*domain.VisitProposal, error)
	updateErr  error
	updated    *domain.VisitProposal
}

func (f *fakeVisitProposalRepo) Save(ctx context.Context, v *domain.VisitProposal) error {
	return errors.New("Save: no debería invocarse en estos tests")
}
func (f *fakeVisitProposalRepo) FindByID(ctx context.Context, id string) (*domain.VisitProposal, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, nil
}
func (f *fakeVisitProposalRepo) Update(ctx context.Context, v *domain.VisitProposal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = v
	return nil
}
func (f *fakeVisitProposalRepo) getUpdated() *domain.VisitProposal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.updated
}

type fakeMessageRepo struct {
	mu      sync.Mutex
	saveErr error
	saved   *domain.Message
}

func (f *fakeMessageRepo) Save(ctx context.Context, m *domain.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = m
	return nil
}
func (f *fakeMessageRepo) FindByConversation(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
	return nil, errors.New("FindByConversation: no debería invocarse en estos tests")
}
func (f *fakeMessageRepo) getSaved() *domain.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saved
}

type fakeConversationRepo struct{}

func (f *fakeConversationRepo) Save(ctx context.Context, c *domain.Conversation) error { return nil }
func (f *fakeConversationRepo) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	return nil, nil
}
func (f *fakeConversationRepo) FindByParticipant(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
	return nil, nil
}
func (f *fakeConversationRepo) FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
	return nil, nil
}

type fakeWebSocketHub struct {
	mu         sync.Mutex
	broadcasts []broadcastCall
}

type broadcastCall struct {
	conversationID string
	payload        []byte
}

func (f *fakeWebSocketHub) Broadcast(conversationID string, payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts = append(f.broadcasts, broadcastCall{conversationID: conversationID, payload: payload})
}
func (f *fakeWebSocketHub) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.broadcasts)
}
func (f *fakeWebSocketHub) lastCall() broadcastCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.broadcasts[len(f.broadcasts)-1]
}

// ─── Helpers ────────────────────────────────────────────────────────────────

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

func newCRMStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "crm",
		Subjects: []string{"crm.>"},
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

func mustProposal(t *testing.T, convID, leadID string) *domain.VisitProposal {
	t.Helper()
	vp, err := domain.NewVisitProposal(convID, leadID, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}
	return vp
}

// ─── StartConsume: errores de arranque ─────────────────────────────────────

func TestStartConsume_StreamInexistente_RetornaError(t *testing.T) {
	js := newEmbeddedJetStream(t)
	sub := chatnats.NewCRMEventSubscriber(js, &fakeVisitProposalRepo{}, &fakeMessageRepo{}, &fakeConversationRepo{}, &fakeWebSocketHub{})

	err := sub.StartConsume(context.Background())

	if err == nil {
		t.Fatal("StartConsume: esperaba un error al no existir el stream 'crm'")
	}
}

func TestStartConsume_RetornaRapidoSinBloquear(t *testing.T) {
	// El consumo corre en una goroutine en background — StartConsume no debe
	// bloquear esperando mensajes (a diferencia de los subscribers de catalog).
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	sub := chatnats.NewCRMEventSubscriber(js, &fakeVisitProposalRepo{}, &fakeMessageRepo{}, &fakeConversationRepo{}, &fakeWebSocketHub{})

	done := make(chan error, 1)
	go func() { done <- sub.StartConsume(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartConsume: error inesperado: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartConsume no debería bloquear — el consumo corre en background")
	}
}

// ─── visit_scheduled ────────────────────────────────────────────────────────

func TestStartConsume_VisitScheduled_HappyPath(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool { return msgRepo.getSaved() != nil })

	if proposalRepo.getUpdated().Status() != domain.VisitProposalAccepted {
		t.Errorf("proposal.Status(): got %q, want %q", proposalRepo.getUpdated().Status(), domain.VisitProposalAccepted)
	}
	sysMsg := msgRepo.getSaved()
	if sysMsg.ConversationID() != "conv-1" || sysMsg.SenderID() != "system" || sysMsg.Type() != domain.MessageTypeSystem {
		t.Errorf("mensaje de sistema: got %+v", sysMsg)
	}
	if sysMsg.Metadata()["status"] != "ACCEPTED" {
		t.Errorf("Metadata: got %v", sysMsg.Metadata())
	}

	waitUntil(t, time.Second, func() bool { return hub.callCount() > 0 })
	bc := hub.lastCall()
	if bc.conversationID != "conv-1" || !strings.Contains(string(bc.payload), "VISIT_CONFIRMED") {
		t.Errorf("Broadcast: got %+v", bc)
	}
}

func TestStartConsume_VisitScheduled_JSONMalformado_NoRompeElConsumidor(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", []byte("esto no es json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // le da tiempo a descartar el mensaje malformado

	// El consumidor debe seguir vivo: un mensaje válido después del malformado se procesa igual.
	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return msgRepo.getSaved() != nil })
}

func TestStartConsume_VisitScheduled_ErrorBuscandoPropuesta_SeTrataComoNoEncontrada(t *testing.T) {
	// OJO: el código trata "error real" y "no encontrada" exactamente igual
	// ("if err != nil || proposal == nil { log; return nil }") — un fallo
	// transitorio de la DB provoca que el mensaje se ackee como si nunca
	// hubiese existido la propuesta, sin reintento posible. Este test fija
	// ese comportamiento actual.
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	boom := errors.New("timeout de base de datos")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return nil, boom },
	}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": "vp-1", "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if msgRepo.getSaved() != nil {
		t.Error("no debería haberse inyectado ningún mensaje de sistema — la propuesta 'no se encontró' (por el error tratado como tal)")
	}
	if hub.callCount() != 0 {
		t.Error("no debería haber broadcast alguno")
	}
}

func TestStartConsume_VisitScheduled_ErrorActualizandoPropuesta_NoInyectaMensaje(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
		updateErr:  errors.New("fallo de escritura"),
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if msgRepo.getSaved() != nil {
		t.Error("el mensaje de sistema no debería inyectarse si falló la actualización de la propuesta")
	}
}

func TestStartConsume_VisitScheduled_ErrorGuardandoMensaje_NoHaceBroadcast(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{saveErr: errors.New("fallo de escritura")}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return proposalRepo.getUpdated() != nil })
	time.Sleep(200 * time.Millisecond)

	if hub.callCount() != 0 {
		t.Error("no debería haber broadcast si falló el guardado del mensaje de sistema")
	}
}

// ─── visit_rejected ─────────────────────────────────────────────────────────

func TestStartConsume_VisitRejected_HappyPath(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "reason": "El propietario canceló",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool { return msgRepo.getSaved() != nil })

	if proposalRepo.getUpdated().Status() != domain.VisitProposalRejected {
		t.Errorf("proposal.Status(): got %q, want %q", proposalRepo.getUpdated().Status(), domain.VisitProposalRejected)
	}
	sysMsg := msgRepo.getSaved()
	if sysMsg.Metadata()["status"] != "REJECTED" || sysMsg.Metadata()["reason"] != "El propietario canceló" {
		t.Errorf("Metadata: got %v", sysMsg.Metadata())
	}

	waitUntil(t, time.Second, func() bool { return hub.callCount() > 0 })
	bc := hub.lastCall()
	if !strings.Contains(string(bc.payload), "VISIT_REJECTED") {
		t.Errorf("Broadcast payload: got %q, want que incluya VISIT_REJECTED", bc.payload)
	}
}

func TestStartConsume_VisitRejected_JSONMalformado_NoRompeElConsumidor(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", []byte("esto no es json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// El consumidor debe seguir vivo tras descartar el mensaje malformado.
	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "reason": "motivo",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return msgRepo.getSaved() != nil })
}

func TestStartConsume_VisitRejected_ErrorBuscandoPropuesta_SeTrataComoNoEncontrada(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	boom := errors.New("timeout de base de datos")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return nil, boom },
	}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": "vp-1", "reason": "motivo",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if msgRepo.getSaved() != nil {
		t.Error("no debería haberse inyectado ningún mensaje de sistema — mismo tratamiento que 'no encontrada'")
	}
	if hub.callCount() != 0 {
		t.Error("no debería haber broadcast alguno")
	}
}

func TestStartConsume_VisitRejected_ErrorActualizandoPropuesta_NoInyectaMensaje(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
		updateErr:  errors.New("fallo de escritura"),
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "reason": "motivo",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if msgRepo.getSaved() != nil {
		t.Error("el mensaje de sistema no debería inyectarse si falló la actualización de la propuesta")
	}
}

func TestStartConsume_VisitRejected_ErrorGuardandoMensaje_NoHaceBroadcast(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{saveErr: errors.New("fallo de escritura")}
	hub := &fakeWebSocketHub{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, hub)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "reason": "motivo",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_rejected", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool { return proposalRepo.getUpdated() != nil })
	time.Sleep(200 * time.Millisecond)

	if hub.callCount() != 0 {
		t.Error("no debería haber broadcast si falló el guardado del mensaje de sistema")
	}
}

// ─── subject no manejado ────────────────────────────────────────────────────

func TestStartConsume_SubjectNoManejado_NoTocaLosRepos(t *testing.T) {
	// "crm.lead.visit_*" matchea el filtro del consumer, pero el switch solo
	// maneja "visit_scheduled" y "visit_rejected" explícitamente — cualquier
	// otro subject cae al default implícito (processErr queda nil, se ackea
	// sin hacer nada).
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) {
			t.Error("FindByID no debería invocarse para un subject no manejado")
			return nil, nil
		},
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	if _, err := js.Publish(context.Background(), "crm.lead.visit_cancelled", []byte(`{}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if msgRepo.getSaved() != nil {
		t.Error("no debería haberse guardado ningún mensaje para un subject no manejado")
	}
}

// ─── Cancelación del contexto ───────────────────────────────────────────────

func TestStartConsume_CancelarElContexto_NoDetieneElConsumo(t *testing.T) {
	// A diferencia de los subscribers de catalog (que registran una goroutine
	// "<-ctx.Done(); iter.Stop()"), CRMEventSubscriber.StartConsume no tiene
	// NINGÚN mecanismo de cancelación: ni defer iter.Stop() ni una goroutine
	// que observe ctx.Done(). Este test demuestra que cancelar el contexto
	// pasado a StartConsume no tiene ningún efecto sobre el consumo en curso —
	// probablemente un descuido a corregir (fuga de goroutine/consumer).
	js := newEmbeddedJetStream(t)
	newCRMStream(t, js)
	proposal := mustProposal(t, "conv-1", "lead-1")
	proposalRepo := &fakeVisitProposalRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.VisitProposal, error) { return proposal, nil },
	}
	msgRepo := &fakeMessageRepo{}
	sub := chatnats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, &fakeConversationRepo{}, &fakeWebSocketHub{})

	ctx, cancel := context.WithCancel(context.Background())
	if err := sub.StartConsume(ctx); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}
	cancel() // cancelamos inmediatamente después de arrancar

	payload, _ := json.Marshal(map[string]string{
		"lead_id": "lead-1", "conversation_id": "conv-1", "proposal_id": proposal.ID(), "scheduled_at": "2026-08-01T10:00:00Z",
	})
	if _, err := js.Publish(context.Background(), "crm.lead.visit_scheduled", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Si hubiese algún mecanismo de cancelación, este mensaje NUNCA se procesaría.
	// Como no lo hay, se procesa exactamente igual que si ctx nunca se hubiese cancelado.
	waitUntil(t, 3*time.Second, func() bool { return msgRepo.getSaved() != nil })
}
