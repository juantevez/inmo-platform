package domain_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// idPattern valida el formato "8-4-4-4-12" hexadecimal que arma nextID()
// internamente (mismo esquema que se usa en otros contextos del monorepo).
var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeBadRequest {
		t.Fatalf("got %v, want AppError BadRequest", err)
	}
}

func assertPreconditionFailed(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypePreconditionFail {
		t.Fatalf("got %v, want AppError PreconditionFailed", err)
	}
}

// ─── NewConversation ────────────────────────────────────────────────────────

func TestNewConversation_CamposObligatoriosFaltantes(t *testing.T) {
	cases := []struct {
		name                               string
		propertyID, seekerID, advertiserID string
	}{
		{"propertyID vacío", "", "seeker-1", "advertiser-1"},
		{"seekerID vacío", "prop-1", "", "advertiser-1"},
		{"advertiserID vacío", "prop-1", "seeker-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewConversation(tc.propertyID, "Depto", tc.seekerID, "Juan", tc.advertiserID, "Ana")
			assertBadRequest(t, err)
		})
	}
}

func TestNewConversation_SeekerIgualAdvertiser_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewConversation("prop-1", "Depto", "user-1", "Juan", "user-1", "Juan")
	assertBadRequest(t, err)
}

func TestNewConversation_HappyPath_MapeaCamposYRegistraEvento(t *testing.T) {
	before := time.Now()
	conv, err := domain.NewConversation("prop-1", "Depto en Palermo", "seeker-1", "Juan", "advertiser-1", "Ana")
	after := time.Now()

	if err != nil {
		t.Fatalf("NewConversation: error inesperado: %v", err)
	}
	if conv.PropertyID() != "prop-1" || conv.PropertyTitle() != "Depto en Palermo" ||
		conv.SeekerID() != "seeker-1" || conv.SeekerName() != "Juan" ||
		conv.AdvertiserID() != "advertiser-1" || conv.AdvertiserName() != "Ana" {
		t.Errorf("conversation: got %+v", conv)
	}
	if conv.LeadID() != "" {
		t.Errorf("LeadID: got %q, want vacío al crear", conv.LeadID())
	}
	if !idPattern.MatchString(conv.ID()) {
		t.Errorf("ID: got %q, no matchea el formato esperado", conv.ID())
	}
	if conv.CreatedAt().Before(before) || conv.CreatedAt().After(after) {
		t.Errorf("CreatedAt: got %v, want entre %v y %v", conv.CreatedAt(), before, after)
	}
	if !conv.CreatedAt().Equal(conv.UpdatedAt()) {
		t.Errorf("CreatedAt/UpdatedAt: got %v / %v, want iguales al crear", conv.CreatedAt(), conv.UpdatedAt())
	}

	events := conv.PullEvents()
	if len(events) != 1 {
		t.Fatalf("eventos: got %d, want 1 (ConversationStarted)", len(events))
	}
	evt, ok := events[0].(domain.ConversationStartedEvent)
	if !ok {
		t.Fatalf("evento: got %T, want ConversationStartedEvent", events[0])
	}
	if evt.EventName() != "chat.conversation.started" || evt.AggregateID() != conv.ID() ||
		evt.PropertyID != "prop-1" || evt.SeekerID != "seeker-1" || evt.AdvertiserID != "advertiser-1" {
		t.Errorf("evento: got %+v", evt)
	}
}

// ─── ReconstructConversation ────────────────────────────────────────────────

func TestReconstructConversation_BypaseaValidacionesYNoRegistraEventos(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Reconstruct permite incluso seeker==advertiser (que NewConversation rechazaría) —
	// es responsabilidad exclusiva de la hidratación desde infraestructura.
	conv := domain.ReconstructConversation("conv-1", "prop-1", "Depto", "user-1", "Juan", "user-1", "Juan", "lead-1", createdAt, updatedAt)

	if conv.ID() != "conv-1" || conv.LeadID() != "lead-1" {
		t.Errorf("got ID=%q LeadID=%q", conv.ID(), conv.LeadID())
	}
	if !conv.CreatedAt().Equal(createdAt) || !conv.UpdatedAt().Equal(updatedAt) {
		t.Errorf("timestamps: got CreatedAt=%v UpdatedAt=%v, want los originales", conv.CreatedAt(), conv.UpdatedAt())
	}
	if len(conv.PullEvents()) != 0 {
		t.Error("ReconstructConversation no debería registrar eventos")
	}
}

// ─── LinkLead ───────────────────────────────────────────────────────────────

func TestLinkLead_AsignaLeadIDYActualizaUpdatedAt(t *testing.T) {
	conv, err := domain.NewConversation("prop-1", "Depto", "seeker-1", "Juan", "advertiser-1", "Ana")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}
	conv.PullEvents() // drena el evento de creación, no es relevante acá
	originalUpdatedAt := conv.UpdatedAt()

	time.Sleep(time.Millisecond) // asegura que time.Now() avance para la comparación
	conv.LinkLead("lead-1")

	if conv.LeadID() != "lead-1" {
		t.Errorf("LeadID: got %q, want %q", conv.LeadID(), "lead-1")
	}
	if !conv.UpdatedAt().After(originalUpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want posterior a %v", conv.UpdatedAt(), originalUpdatedAt)
	}
}

// ─── IsParticipant ──────────────────────────────────────────────────────────

func TestIsParticipant(t *testing.T) {
	conv, err := domain.NewConversation("prop-1", "Depto", "seeker-1", "Juan", "advertiser-1", "Ana")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}

	cases := []struct {
		userID string
		want   bool
	}{
		{"seeker-1", true},
		{"advertiser-1", true},
		{"extraño-1", false},
	}
	for _, tc := range cases {
		if got := conv.IsParticipant(tc.userID); got != tc.want {
			t.Errorf("IsParticipant(%q): got %v, want %v", tc.userID, got, tc.want)
		}
	}
}

// ─── Touch ──────────────────────────────────────────────────────────────────

func TestTouch_ActualizaSoloUpdatedAt(t *testing.T) {
	conv, err := domain.NewConversation("prop-1", "Depto", "seeker-1", "Juan", "advertiser-1", "Ana")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}
	originalCreatedAt := conv.CreatedAt()
	originalUpdatedAt := conv.UpdatedAt()

	time.Sleep(time.Millisecond)
	conv.Touch()

	if !conv.CreatedAt().Equal(originalCreatedAt) {
		t.Errorf("CreatedAt no debería cambiar: got %v, want %v", conv.CreatedAt(), originalCreatedAt)
	}
	if !conv.UpdatedAt().After(originalUpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want posterior a %v", conv.UpdatedAt(), originalUpdatedAt)
	}
}

// ─── NewTextMessage ─────────────────────────────────────────────────────────

func TestNewTextMessage_CamposObligatoriosFaltantes(t *testing.T) {
	cases := []struct {
		name                     string
		conversationID, senderID string
	}{
		{"conversationID vacío", "", "sender-1"},
		{"senderID vacío", "conv-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewTextMessage(tc.conversationID, tc.senderID, "Hola")
			assertBadRequest(t, err)
		})
	}
}

func TestNewTextMessage_BodyVacio_RetornaBadRequest(t *testing.T) {
	_, err := domain.NewTextMessage("conv-1", "sender-1", "")
	assertBadRequest(t, err)
}

func TestNewTextMessage_BodyDemasiadoLargo_RetornaBadRequest(t *testing.T) {
	body := strings.Repeat("a", 4001)
	_, err := domain.NewTextMessage("conv-1", "sender-1", body)
	assertBadRequest(t, err)
}

func TestNewTextMessage_BodyEnElLimiteExacto_EsValido(t *testing.T) {
	body := strings.Repeat("a", 4000)
	msg, err := domain.NewTextMessage("conv-1", "sender-1", body)

	if err != nil {
		t.Fatalf("NewTextMessage: error inesperado con 4000 caracteres: %v", err)
	}
	if len(msg.Body()) != 4000 {
		t.Errorf("Body length: got %d, want 4000", len(msg.Body()))
	}
}

func TestNewTextMessage_HappyPath(t *testing.T) {
	msg, err := domain.NewTextMessage("conv-1", "sender-1", "Hola, ¿sigue disponible?")

	if err != nil {
		t.Fatalf("NewTextMessage: error inesperado: %v", err)
	}
	if msg.ConversationID() != "conv-1" || msg.SenderID() != "sender-1" || msg.Body() != "Hola, ¿sigue disponible?" {
		t.Errorf("message: got %+v", msg)
	}
	if msg.Type() != domain.MessageTypeText {
		t.Errorf("Type: got %q, want %q", msg.Type(), domain.MessageTypeText)
	}
	if msg.Metadata() == nil || len(msg.Metadata()) != 0 {
		t.Errorf("Metadata: got %v, want mapa vacío no-nil", msg.Metadata())
	}
	if !idPattern.MatchString(msg.ID()) {
		t.Errorf("ID: got %q, no matchea el formato esperado", msg.ID())
	}
}

// ─── NewVisitProposalMessage ────────────────────────────────────────────────

func TestNewVisitProposalMessage_CamposObligatoriosFaltantes(t *testing.T) {
	cases := []struct {
		name                                                  string
		conversationID, senderID, visitProposalID, proposedAt string
	}{
		{"conversationID vacío", "", "sender-1", "vp-1", "2025-01-01T10:00:00Z"},
		{"senderID vacío", "conv-1", "", "vp-1", "2025-01-01T10:00:00Z"},
		{"visitProposalID vacío", "conv-1", "sender-1", "", "2025-01-01T10:00:00Z"},
		{"proposedAt vacío", "conv-1", "sender-1", "vp-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewVisitProposalMessage(tc.conversationID, tc.senderID, tc.visitProposalID, tc.proposedAt)
			assertBadRequest(t, err)
		})
	}
}

func TestNewVisitProposalMessage_HappyPath(t *testing.T) {
	msg, err := domain.NewVisitProposalMessage("conv-1", "sender-1", "vp-1", "2025-01-01T10:00:00Z")

	if err != nil {
		t.Fatalf("NewVisitProposalMessage: error inesperado: %v", err)
	}
	if msg.Type() != domain.MessageTypeVisitProposal {
		t.Errorf("Type: got %q, want %q", msg.Type(), domain.MessageTypeVisitProposal)
	}
	if msg.Body() != "Propuesta de visita" {
		t.Errorf("Body: got %q", msg.Body())
	}
	meta := msg.Metadata()
	if meta["visit_proposal_id"] != "vp-1" || meta["proposed_at"] != "2025-01-01T10:00:00Z" ||
		meta["status"] != string(domain.VisitProposalPending) {
		t.Errorf("Metadata: got %v", meta)
	}
}

// ─── NewSystemMessage ───────────────────────────────────────────────────────

func TestNewSystemMessage_MetaNil_UsaMapaVacio(t *testing.T) {
	msg := domain.NewSystemMessage("conv-1", "Visita confirmada", nil)

	if msg.Metadata() == nil || len(msg.Metadata()) != 0 {
		t.Errorf("Metadata: got %v, want mapa vacío no-nil", msg.Metadata())
	}
	if msg.SenderID() != "system" {
		t.Errorf("SenderID: got %q, want %q", msg.SenderID(), "system")
	}
	if msg.Type() != domain.MessageTypeSystem {
		t.Errorf("Type: got %q, want %q", msg.Type(), domain.MessageTypeSystem)
	}
}

func TestNewSystemMessage_ConMeta_SePreserva(t *testing.T) {
	meta := map[string]string{"visit_proposal_id": "vp-1"}
	msg := domain.NewSystemMessage("conv-1", "Visita confirmada", meta)

	if msg.Metadata()["visit_proposal_id"] != "vp-1" {
		t.Errorf("Metadata: got %v", msg.Metadata())
	}
	if msg.Body() != "Visita confirmada" {
		t.Errorf("Body: got %q", msg.Body())
	}
}

// ─── ReconstructMessage ─────────────────────────────────────────────────────

func TestReconstructMessage_MetadataNil_UsaMapaVacio(t *testing.T) {
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	msg := domain.ReconstructMessage("msg-1", "conv-1", "sender-1", domain.MessageTypeText, "Hola", nil, createdAt)

	if msg.Metadata() == nil || len(msg.Metadata()) != 0 {
		t.Errorf("Metadata: got %v, want mapa vacío no-nil", msg.Metadata())
	}
	if !msg.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt: got %v, want %v", msg.CreatedAt(), createdAt)
	}
}

func TestReconstructMessage_BypaseaValidaciones(t *testing.T) {
	// Acepta un msgType arbitrario y un body vacío — cosas que NewTextMessage rechazaría.
	msg := domain.ReconstructMessage("msg-1", "conv-1", "sender-1", domain.MessageType("LEGACY"), "", nil, time.Now())

	if msg.Type() != domain.MessageType("LEGACY") {
		t.Errorf("Type: got %q, want se preserve sin validar", msg.Type())
	}
}

// ─── NewVisitProposal ───────────────────────────────────────────────────────

func TestNewVisitProposal_CamposObligatoriosFaltantes(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	cases := []struct {
		name                   string
		conversationID, leadID string
	}{
		{"conversationID vacío", "", "lead-1"},
		{"leadID vacío", "conv-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewVisitProposal(tc.conversationID, tc.leadID, future)
			assertBadRequest(t, err)
		})
	}
}

func TestNewVisitProposal_FechaEnElPasado_RetornaBadRequest(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	_, err := domain.NewVisitProposal("conv-1", "lead-1", past)
	assertBadRequest(t, err)
}

func TestNewVisitProposal_HappyPath(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", future)

	if err != nil {
		t.Fatalf("NewVisitProposal: error inesperado: %v", err)
	}
	if vp.ConversationID() != "conv-1" || vp.LeadID() != "lead-1" {
		t.Errorf("got ConversationID=%q LeadID=%q", vp.ConversationID(), vp.LeadID())
	}
	if vp.Status() != domain.VisitProposalPending {
		t.Errorf("Status: got %q, want %q", vp.Status(), domain.VisitProposalPending)
	}
	if vp.ResolvedAt() != nil {
		t.Errorf("ResolvedAt: got %v, want nil", vp.ResolvedAt())
	}
	if !vp.ProposedAt().Equal(future) {
		t.Errorf("ProposedAt: got %v, want %v", vp.ProposedAt(), future)
	}
}

// ─── Accept / Reject ────────────────────────────────────────────────────────

func TestVisitProposal_Accept_DesdePending_Exito(t *testing.T) {
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}

	if err := vp.Accept(); err != nil {
		t.Fatalf("Accept: error inesperado: %v", err)
	}
	if vp.Status() != domain.VisitProposalAccepted {
		t.Errorf("Status: got %q, want %q", vp.Status(), domain.VisitProposalAccepted)
	}
	if vp.ResolvedAt() == nil {
		t.Error("ResolvedAt: got nil, want seteado tras Accept")
	}
}

func TestVisitProposal_Accept_YaResuelta_RetornaPreconditionFailed(t *testing.T) {
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}
	if err := vp.Accept(); err != nil {
		t.Fatalf("setup Accept: %v", err)
	}

	err = vp.Accept()
	assertPreconditionFailed(t, err)
}

func TestVisitProposal_Reject_DesdePending_Exito(t *testing.T) {
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}

	if err := vp.Reject(); err != nil {
		t.Fatalf("Reject: error inesperado: %v", err)
	}
	if vp.Status() != domain.VisitProposalRejected {
		t.Errorf("Status: got %q, want %q", vp.Status(), domain.VisitProposalRejected)
	}
	if vp.ResolvedAt() == nil {
		t.Error("ResolvedAt: got nil, want seteado tras Reject")
	}
}

func TestVisitProposal_Reject_YaResuelta_RetornaPreconditionFailed(t *testing.T) {
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}
	if err := vp.Reject(); err != nil {
		t.Fatalf("setup Reject: %v", err)
	}

	err = vp.Reject()
	assertPreconditionFailed(t, err)
}

// ─── ReconstructVisitProposal ───────────────────────────────────────────────

func TestReconstructVisitProposal_PreservaValoresCrudos(t *testing.T) {
	proposedAt := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	resolvedAt := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)

	vp := domain.ReconstructVisitProposal("vp-1", "conv-1", "lead-1", proposedAt, domain.VisitProposalAccepted, &resolvedAt, createdAt)

	if vp.ID() != "vp-1" || vp.Status() != domain.VisitProposalAccepted {
		t.Errorf("got ID=%q Status=%q", vp.ID(), vp.Status())
	}
	if vp.ResolvedAt() == nil || !vp.ResolvedAt().Equal(resolvedAt) {
		t.Errorf("ResolvedAt: got %v, want %v", vp.ResolvedAt(), resolvedAt)
	}
	if !vp.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt: got %v, want %v", vp.CreatedAt(), createdAt)
	}
}
