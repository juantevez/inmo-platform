package nats_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	crmnats "inmo.platform/contexts/crm/internal/adapters/nats"
	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// PropertyEventSubscriber recibe un *application.CreateAutoLeadUseCase
// concreto (no una interfaz), así que para observar su comportamiento
// construimos el caso de uso real sobre un fake de ports.LeadRepository y
// espiamos las llamadas a Save.

type fakeLeadRepo struct {
	mu      sync.Mutex
	saveFn  func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error
	savedN  int
	lastArg *domain.Lead
}

func (f *fakeLeadRepo) Save(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
	f.mu.Lock()
	f.savedN++
	f.lastArg = lead
	f.mu.Unlock()
	if f.saveFn != nil {
		return f.saveFn(ctx, lead, eventName, eventPayload)
	}
	return nil
}

func (f *fakeLeadRepo) GetByID(ctx context.Context, id string) (*domain.Lead, error) {
	return nil, errors.New("fixture no configurada")
}

func (f *fakeLeadRepo) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.savedN
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

func newCatalogStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "catalog",
		Subjects: []string{"catalog.property.*"},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
}

func publishJSON(t *testing.T, js jetstream.JetStream, subject, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := js.Publish(ctx, subject, []byte(payload)); err != nil {
		t.Fatalf("Publish: %v", err)
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

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestPropertyEventSubscriber_StreamCatalogNoExiste_RetornaError(t *testing.T) {
	js := newEmbeddedJetStream(t) // sin crear el stream "catalog"
	repo := &fakeLeadRepo{}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	err := sub.StartConsume(context.Background())
	if err == nil {
		t.Fatal("esperaba un error si el stream 'catalog' no existe")
	}
}

func TestPropertyEventSubscriber_EventoValido_CreaElLeadYNoReintenta(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	repo := &fakeLeadRepo{}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	publishJSON(t, js, "catalog.property.published", `{"aggregate_id":"prop-1","owner_id":"owner-1"}`)

	waitUntil(t, 3*time.Second, func() bool { return repo.callCount() >= 1 })

	if repo.lastArg.PropertyID != "prop-1" {
		t.Fatalf("property_id: got %s, want prop-1", repo.lastArg.PropertyID)
	}
	if repo.lastArg.State != domain.StateNew {
		t.Fatalf("state: got %s, want %s", repo.lastArg.State, domain.StateNew)
	}

	// Al haberse Ackeado con éxito, no debería redeliverse: el conteo se
	// mantiene estable pasado un rato.
	time.Sleep(300 * time.Millisecond)
	if got := repo.callCount(); got != 1 {
		t.Fatalf("esperaba exactamente 1 llamada a Save (sin redelivery tras el Ack), hubo %d", got)
	}
}

func TestPropertyEventSubscriber_MensajeToxico_NuncaEjecutaElCasoDeUso(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	repo := &fakeLeadRepo{}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	publishJSON(t, js, "catalog.property.published", `{"aggregate_id":"prop-error-poison","owner_id":"owner-1"}`)

	// Nak dispara redelivery casi inmediata (no respeta AckWait) — esperamos
	// un rato prudencial y confirmamos que el caso de uso jamás se ejecuta,
	// ni siquiera en los reintentos.
	time.Sleep(500 * time.Millisecond)
	if got := repo.callCount(); got != 0 {
		t.Fatalf("un mensaje tóxico no debería llegar a ejecutar el caso de uso: %d llamadas", got)
	}
}

func TestPropertyEventSubscriber_JSONMalformado_NuncaEjecutaElCasoDeUso(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	repo := &fakeLeadRepo{}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	publishJSON(t, js, "catalog.property.published", `{esto no es json valido`)

	// Term() descarta el mensaje definitivamente (no se puede arreglar
	// reintentando) — confirmamos que nunca se llega al caso de uso.
	time.Sleep(500 * time.Millisecond)
	if got := repo.callCount(); got != 0 {
		t.Fatalf("un JSON malformado no debería llegar a ejecutar el caso de uso: %d llamadas", got)
	}
}

func TestPropertyEventSubscriber_PropertyIDVacio_ElCasoDeUsoFallaYReintenta(t *testing.T) {
	// aggregate_id vacío -> domain.NewLead rechaza el lead (propertyID
	// obligatorio) -> el caso de uso retorna error antes de llegar a
	// repo.Save -> el subscriber hace Nak.
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	repo := &fakeLeadRepo{}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	publishJSON(t, js, "catalog.property.published", `{"aggregate_id":"","owner_id":"owner-1"}`)

	time.Sleep(500 * time.Millisecond)
	if got := repo.callCount(); got != 0 {
		t.Fatalf("con propertyID vacío, domain.NewLead debería fallar antes de llegar a Save: %d llamadas", got)
	}
}

func TestPropertyEventSubscriber_ErrorAlGuardar_ReintentaConNak(t *testing.T) {
	js := newEmbeddedJetStream(t)
	newCatalogStream(t, js)
	repo := &fakeLeadRepo{
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			return errors.New("fallo de escritura")
		},
	}
	uc := application.NewCreateAutoLeadUseCase(repo)
	sub := crmnats.NewPropertyEventSubscriber(js, uc)

	if err := sub.StartConsume(context.Background()); err != nil {
		t.Fatalf("StartConsume: %v", err)
	}

	publishJSON(t, js, "catalog.property.published", `{"aggregate_id":"prop-1","owner_id":"owner-1"}`)

	// Si Save falla, el subscriber hace Nak y NATS redelivera — confirmamos
	// que se reintenta más de una vez (MaxDeliver: 3).
	waitUntil(t, 5*time.Second, func() bool { return repo.callCount() >= 2 })
}
