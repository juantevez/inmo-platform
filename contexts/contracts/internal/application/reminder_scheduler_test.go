package application_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"inmo.platform/contexts/contracts/internal/application"
	"inmo.platform/contexts/contracts/internal/domain"
)

// ─── Helpers ────────────────────────────────────────────────────────────────
//
// ReminderScheduler lee GATEWAY_URL de una variable de entorno al construirse
// (no es inyectable), así que apuntamos ese gateway a un httptest.NewServer
// que simula el chat service — mismo patrón usado para
// ReservationNotificationSubscriber en el paquete nats.

type recordedReminderRequest struct {
	path   string
	header http.Header
	body   map[string]any
}

type fakeChatGateway struct {
	mu            sync.Mutex
	requests      []recordedReminderRequest
	resolveStatus int
	resolveBody   string // si vacío, responde {"id":"chat-1"}
	messageStatus int
}

func newFakeChatGateway() *fakeChatGateway {
	return &fakeChatGateway{resolveStatus: http.StatusOK, messageStatus: http.StatusOK}
}

func (g *fakeChatGateway) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/chats", func(w http.ResponseWriter, r *http.Request) {
		g.record(t, r)
		if g.resolveStatus != http.StatusOK {
			w.WriteHeader(g.resolveStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := g.resolveBody
		if body == "" {
			body = `{"id":"chat-1"}`
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		g.record(t, r)
		w.WriteHeader(g.messageStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (g *fakeChatGateway) record(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]any
	data, _ := io.ReadAll(r.Body)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &body)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, recordedReminderRequest{path: r.URL.Path, header: r.Header.Clone(), body: body})
}

func (g *fakeChatGateway) countPath(path string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, r := range g.requests {
		if r.path == path {
			n++
		}
	}
	return n
}

func (g *fakeChatGateway) firstWithPath(path string) recordedReminderRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, r := range g.requests {
		if r.path == path {
			return r
		}
	}
	return recordedReminderRequest{}
}

func waitUntilReminder(t *testing.T, timeout time.Duration, cond func() bool) {
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

func runReminderStartAndCancel(t *testing.T, rs *application.ReminderScheduler, waitFor func() bool, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rs.Start(ctx)
		close(done)
	}()

	if waitFor != nil {
		waitUntilReminder(t, timeout, waitFor)
	} else {
		time.Sleep(150 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start no retornó tras cancelar el contexto")
	}
}

func confirmedReservationDueTomorrow(t *testing.T, id string) *domain.Reservation {
	t.Helper()
	checkIn := time.Now().Add(24 * time.Hour)
	return domain.ReconstructReservation(id, "prop-1", "tenant-1", "owner-1",
		checkIn, checkIn.Add(3*24*time.Hour), 3, 10000, 0, 1500, 5000, 31500,
		domain.ReservationConfirmed, "", nil, nil, time.Now(), time.Now())
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestReminderScheduler_TickerPeriodico_EscaneaMasDeUnaVez(t *testing.T) {
	// Start() escanea inmediatamente al arrancar y de nuevo en cada tick del
	// ticker. Con el interval default (1h) solo se ejercita el primer scan;
	// acá lo acortamos vía REMINDER_POLL_INTERVAL para confirmar que el
	// segundo scan también ocurre.
	t.Setenv("REMINDER_POLL_INTERVAL", "20ms")
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	var scans int32
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			atomic.AddInt32(&scans, 1)
			return nil, nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return atomic.LoadInt32(&scans) >= 2 }, 3*time.Second)
}

func TestReminderScheduler_CancelacionDeContexto_TerminaLimpiamente(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return nil, nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, nil, 0)
}

func TestReminderScheduler_ErrorAlBuscarReservas_NoRompeYNoLlamaAlGateway(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return nil, errFixtureNoConfigurada
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, nil, 0)

	if gw.countPath("/api/v1/chats") != 0 {
		t.Fatalf("no debería haber llamado al gateway: %d intentos", gw.countPath("/api/v1/chats"))
	}
}

func TestReminderScheduler_SinReservasProximas_NoLlamaAlGateway(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return nil, nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, nil, 0)

	if gw.countPath("/api/v1/chats") != 0 {
		t.Fatalf("no debería haber llamado al gateway: %d intentos", gw.countPath("/api/v1/chats"))
	}
}

func TestReminderScheduler_ReservaProxima_EnviaRecordatorioYMarcaComoEnviada(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	var capturedFrom, capturedTo time.Time
	var markedID string
	var markMu sync.Mutex

	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			capturedFrom, capturedTo = from, to
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			markMu.Lock()
			markedID = reservationID
			markMu.Unlock()
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool {
		markMu.Lock()
		defer markMu.Unlock()
		return markedID == "res-1"
	}, 3*time.Second)

	if gw.countPath("/api/v1/chats") == 0 {
		t.Fatal("esperaba al menos un intento de resolveChat")
	}
	if gw.countPath("/api/v1/chats/chat-1/messages") == 0 {
		t.Fatal("esperaba al menos un intento de postear el mensaje de recordatorio")
	}

	msgReq := gw.firstWithPath("/api/v1/chats/chat-1/messages")
	if msgReq.body["message_type"] != "system" {
		t.Fatalf("message_type: got %v, want system", msgReq.body["message_type"])
	}
	text, _ := msgReq.body["body"].(string)
	if text == "" {
		t.Fatal("el body del recordatorio no debería estar vacío")
	}

	// from y to se calculan con dos llamadas independientes a time.Now(), así
	// que puede haber una diferencia de nanosegundos entre ellas.
	if delta := capturedTo.Sub(capturedFrom) - 2*time.Hour; delta < 0 || delta > time.Millisecond {
		t.Fatalf("ventana de búsqueda: got %v, want ~2h (from=+23h, to=+25h)", capturedTo.Sub(capturedFrom))
	}
}

func TestReminderScheduler_ResolveChatFalla_NoMarcaComoEnviada(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveStatus = http.StatusInternalServerError
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	markCalled := false
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			markCalled = true
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats") >= 1 }, 3*time.Second)

	if markCalled {
		t.Fatal("no debería marcarse reminder_sent si resolveChat falla")
	}
	if gw.countPath("/api/v1/chats/chat-1/messages") != 0 {
		t.Fatal("no debería intentarse postear el mensaje si resolveChat falló")
	}
}

func TestReminderScheduler_MarkReminderSentFalla_NoRompeElScan(t *testing.T) {
	// El mensaje ya se envió; que falle el "marcado" es best-effort — el
	// scheduler sigue funcionando (se relogueará en el próximo scan, riesgo
	// de duplicado aceptado por diseño, no es objeto de este test).
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			return errFixtureNoConfigurada
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats/chat-1/messages") >= 1 }, 3*time.Second)
}

func TestReminderScheduler_VariasReservas_UnaFallaOtraSeEnvia(t *testing.T) {
	gw := newFakeChatGateway()
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r1 := confirmedReservationDueTomorrow(t, "res-1")
	r2 := confirmedReservationDueTomorrow(t, "res-2")

	var markedIDs []string
	var mu sync.Mutex
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r1, r2}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			mu.Lock()
			markedIDs = append(markedIDs, reservationID)
			mu.Unlock()
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(markedIDs) == 2
	}, 3*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(markedIDs) != 2 {
		t.Fatalf("esperaba que ambas reservas se marquen como enviadas: %v", markedIDs)
	}
}

func TestReminderScheduler_PostearMensajeFalla_NoMarcaComoEnviada(t *testing.T) {
	gw := newFakeChatGateway()
	gw.messageStatus = http.StatusInternalServerError
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	markCalled := false
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			markCalled = true
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats/chat-1/messages") >= 1 }, 3*time.Second)

	if markCalled {
		t.Fatal("no debería marcarse reminder_sent si falla el posteo del mensaje")
	}
}

func TestReminderScheduler_ResolveChatJSONMalformado_NoMarcaComoEnviada(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveBody = `{esto no es json valido`
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	markCalled := false
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			markCalled = true
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats") >= 1 }, 3*time.Second)

	if markCalled {
		t.Fatal("no debería marcarse reminder_sent si resolveChat no pudo decodificar la respuesta")
	}
	if gw.countPath("/api/v1/chats/chat-1/messages") != 0 {
		t.Fatal("no debería intentarse postear el mensaje si resolveChat falló")
	}
}

func TestReminderScheduler_ResolveChatUsaConversationIDComoFallback(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveBody = `{"conversation_id":"conv-1"}` // sin "id", solo el alias
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error { return nil },
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats/conv-1/messages") >= 1 }, 3*time.Second)
}

func TestReminderScheduler_ResolveChatSinIDNiConversationID_NoMarcaComoEnviada(t *testing.T) {
	gw := newFakeChatGateway()
	gw.resolveBody = `{}`
	srv := gw.server(t)
	t.Setenv("GATEWAY_URL", srv.URL)

	r := confirmedReservationDueTomorrow(t, "res-1")
	markCalled := false
	resRepo := &fakeReservationRepo{
		findConfirmedCheckingInBetweenFn: func(ctx context.Context, from, to time.Time) ([]*domain.Reservation, error) {
			return []*domain.Reservation{r}, nil
		},
		markReminderSentFn: func(ctx context.Context, reservationID string) error {
			markCalled = true
			return nil
		},
	}
	rs := application.NewReminderScheduler(resRepo)

	runReminderStartAndCancel(t, rs, func() bool { return gw.countPath("/api/v1/chats") >= 1 }, 3*time.Second)

	if markCalled {
		t.Fatal("no debería marcarse reminder_sent sin un ID de chat válido")
	}
}
