package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────
//
// ProposeVisitUseCase dispara el publish/broadcast en una goroutine ("best
// effort", según el comentario del código) — Execute() retorna sin esperarla.
// Estos fakes son thread-safe y los tests que dependen de ellos usan
// waitUntil para sondear con timeout, ya que no hay ninguna señal síncrona.

type publishCall struct {
	subject string
	payload []byte
}

type fakeEventPublisher struct {
	mu    sync.Mutex
	calls []publishCall
	err   error
}

func (f *fakeEventPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, publishCall{subject: subject, payload: payload})
	return f.err
}

func (f *fakeEventPublisher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeEventPublisher) lastCall() publishCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

type broadcastCall struct {
	conversationID string
	payload        []byte
}

type fakeWebSocketHub struct {
	mu         sync.Mutex
	broadcasts []broadcastCall
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

type fakeVisitProposalRepo struct {
	saveErr   error
	saved     *domain.VisitProposal
	updateErr error
	updated   *domain.VisitProposal
}

func (f *fakeVisitProposalRepo) Save(ctx context.Context, v *domain.VisitProposal) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = v
	return nil
}
func (f *fakeVisitProposalRepo) FindByID(ctx context.Context, id string) (*domain.VisitProposal, error) {
	return nil, errors.New("FindByID: fixture no configurada")
}
func (f *fakeVisitProposalRepo) Update(ctx context.Context, v *domain.VisitProposal) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = v
	return nil
}

// waitUntil sondea la condición hasta que sea verdadera o se agote el timeout.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condición no se cumplió dentro del timeout")
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func newProposeVisitUseCase(
	convRepo *fakeConversationRepo, msgRepo *fakeMessageRepo, proposalRepo *fakeVisitProposalRepo,
	hub *fakeWebSocketHub, publisher *fakeEventPublisher,
) *application.ProposeVisitUseCase {
	return application.NewProposeVisitUseCase(convRepo, msgRepo, proposalRepo, hub, publisher)
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestProposeVisit_ErrorBuscandoConversacion_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, boom },
	}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{ConversationID: "conv-1", SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour)})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestProposeVisit_ConversacionNoExiste_RetornaNotFound(t *testing.T) {
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return nil, nil },
	}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{ConversationID: "no-existe", SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour)})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeNotFound {
		t.Fatalf("Execute: got %v, want AppError NotFound", err)
	}
}

func TestProposeVisit_SolicitanteNoEsElSeeker_RetornaForbidden(t *testing.T) {
	// Ojo: el chequeo es estricto contra SeekerID — ni siquiera el advertiser
	// (que sí es participante de la conversación) puede proponer una visita.
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "advertiser-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour),
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeForbidden {
		t.Fatalf("Execute: got %v, want AppError Forbidden (solo el seeker puede proponer)", err)
	}
}

func TestProposeVisit_FechaPropuestaEnElPasado_RetornaErrorDeDominio(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(-24 * time.Hour),
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("Execute: got %v, want AppError BadRequest (fecha en el pasado)", err)
	}
}

func TestProposeVisit_ErrorGuardandoLaPropuesta_RetornaErrorEnvuelto(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	boom := errors.New("fallo de escritura")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	proposalRepo := &fakeVisitProposalRepo{saveErr: boom}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, proposalRepo, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour),
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
}

func TestProposeVisit_ErrorGuardandoElMensaje_RetornaErrorEnvueltoPeroLaPropuestaYaQuedoGuardada(t *testing.T) {
	// No hay transacción entre proposalRepo.Save y msgRepo.Save — si el segundo
	// falla, la propuesta ya persistió. Documentamos ese comportamiento actual.
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	boom := errors.New("fallo de escritura")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	proposalRepo := &fakeVisitProposalRepo{}
	msgRepo := &fakeMessageRepo{saveErr: boom}
	uc := newProposeVisitUseCase(convRepo, msgRepo, proposalRepo, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour),
	})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want error que envuelva %v", err, boom)
	}
	if proposalRepo.saved == nil {
		t.Error("la propuesta debería haber quedado guardada pese al fallo posterior en el mensaje")
	}
}

func TestProposeVisit_ErrorTocandoLaConversacion_NoFallaLaOperacion(t *testing.T) {
	// El error de convRepo.Save (Touch) se ignora deliberadamente ("_ = ...").
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
		saveErr:    errors.New("fallo al actualizar la conversación"),
	}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour),
	})

	if err != nil {
		t.Fatalf("Execute: got %v, want nil (el fallo de Touch/Save no debe propagarse)", err)
	}
}

func TestProposeVisit_HappyPath_RetornaDTOYPublicaAsincrónicamente(t *testing.T) {
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	proposalRepo := &fakeVisitProposalRepo{}
	msgRepo := &fakeMessageRepo{}
	hub := &fakeWebSocketHub{}
	publisher := &fakeEventPublisher{}
	uc := newProposeVisitUseCase(convRepo, msgRepo, proposalRepo, hub, publisher)

	proposedAt := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	dto, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: proposedAt,
	})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if dto.ConversationID != conv.ID() || dto.LeadID != "lead-1" || dto.Status != string(domain.VisitProposalPending) {
		t.Errorf("VisitProposalDTO: got %+v", dto)
	}
	wantProposedAt := proposedAt.Format("2006-01-02T15:04:05Z")
	if dto.ProposedAt != wantProposedAt {
		t.Errorf("ProposedAt: got %q, want %q", dto.ProposedAt, wantProposedAt)
	}
	if dto.MessageID == "" {
		t.Error("MessageID: no debería estar vacío")
	}
	if proposalRepo.saved == nil || msgRepo.saved == nil {
		t.Fatal("la propuesta y el mensaje deberían haberse persistido")
	}

	// El publish/broadcast corre en una goroutine — sondeamos con timeout.
	waitUntil(t, time.Second, func() bool { return publisher.callCount() > 0 && hub.callCount() > 0 })

	pubCall := publisher.lastCall()
	if pubCall.subject != "chat.visit.proposed" {
		t.Errorf("Publish subject: got %q, want %q", pubCall.subject, "chat.visit.proposed")
	}
	if !strings.Contains(string(pubCall.payload), conv.ID()) || !strings.Contains(string(pubCall.payload), "lead-1") {
		t.Errorf("Publish payload: got %q, want que incluya conversation_id y lead_id", pubCall.payload)
	}

	bcCall := hub.lastCall()
	if bcCall.conversationID != conv.ID() {
		t.Errorf("Broadcast conversationID: got %q, want %q", bcCall.conversationID, conv.ID())
	}
	var wsMsg application.MessageDTO
	if err := json.Unmarshal(bcCall.payload, &wsMsg); err != nil {
		t.Fatalf("Broadcast payload: no es un MessageDTO válido: %v", err)
	}
	if wsMsg.ID != dto.MessageID || wsMsg.Type != string(domain.MessageTypeVisitProposal) {
		t.Errorf("Broadcast MessageDTO: got %+v", wsMsg)
	}
}

func TestProposeVisit_ErrorAlPublicarOBroadcastear_NoAfectaLaRespuesta(t *testing.T) {
	// Ambos son best-effort — un error del publisher no debería impedir que
	// Execute ya haya retornado el DTO exitosamente (la goroutine corre después).
	conv := mustConversation(t, "seeker-1", "advertiser-1")
	convRepo := &fakeConversationRepo{
		findByIDFn: func(ctx context.Context, id string) (*domain.Conversation, error) { return conv, nil },
	}
	publisher := &fakeEventPublisher{err: errors.New("nats no disponible")}
	uc := newProposeVisitUseCase(convRepo, &fakeMessageRepo{}, &fakeVisitProposalRepo{}, &fakeWebSocketHub{}, publisher)

	dto, err := uc.Execute(context.Background(), application.ProposeVisitCommand{
		ConversationID: conv.ID(), SeekerID: "seeker-1", LeadID: "lead-1", ProposedAt: time.Now().Add(24 * time.Hour),
	})

	if err != nil || dto == nil {
		t.Fatalf("Execute: got dto=%v err=%v, want un DTO válido sin error", dto, err)
	}
	waitUntil(t, time.Second, func() bool { return publisher.callCount() > 0 })
}
