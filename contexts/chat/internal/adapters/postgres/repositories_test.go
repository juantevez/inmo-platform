package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/chat/internal/adapters/postgres"
	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func assertInternalError(t *testing.T, err error) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Type != apperr.TypeInternal {
		t.Fatalf("got %v, want AppError Internal", err)
	}
}

func mustMessage(t *testing.T, convID, senderID, body string) *domain.Message {
	t.Helper()
	m, err := domain.NewTextMessage(convID, senderID, body)
	if err != nil {
		t.Fatalf("NewTextMessage: %v", err)
	}
	return m
}

func mustProposal(t *testing.T) *domain.VisitProposal {
	t.Helper()
	vp, err := domain.NewVisitProposal("conv-1", "lead-1", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("NewVisitProposal: %v", err)
	}
	return vp
}

// ─── MessageRepository.Save ─────────────────────────────────────────────────

func TestMessageRepository_Save_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)
	msg := mustMessage(t, "conv-1", "seeker-1", "Hola, ¿sigue disponible?")

	mock.ExpectExec(`INSERT INTO chat_messages`).
		WithArgs(msg.ID(), "conv-1", "seeker-1", string(domain.MessageTypeText), "Hola, ¿sigue disponible?", []byte("{}"), msg.CreatedAt()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Save(context.Background(), msg); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestMessageRepository_Save_ErrorDeEjecucion_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)
	msg := mustMessage(t, "conv-1", "seeker-1", "Hola")

	mock.ExpectExec(`INSERT INTO chat_messages`).WillReturnError(errors.New("fallo de escritura"))

	err := repo.Save(context.Background(), msg)
	assertInternalError(t, err)
}

// ─── MessageRepository.FindByConversation ──────────────────────────────────

var msgColumns = []string{"id", "conversation_id", "sender_id", "msg_type", "body", "metadata", "created_at"}

func TestMessageRepository_FindByConversation_ErrorDeQuery_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)

	mock.ExpectQuery(`SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at`).
		WithArgs("conv-1", 50, 0).
		WillReturnError(errors.New("timeout de base de datos"))

	_, err := repo.FindByConversation(context.Background(), "conv-1", 50, 0)
	assertInternalError(t, err)
}

func TestMessageRepository_FindByConversation_SinResultados_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)

	mock.ExpectQuery(`SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at`).
		WithArgs("conv-1", 50, 0).
		WillReturnRows(sqlmock.NewRows(msgColumns))

	msgs, err := repo.FindByConversation(context.Background(), "conv-1", 50, 0)

	if err != nil {
		t.Fatalf("FindByConversation: error inesperado: %v", err)
	}
	// A diferencia de otros repos del monorepo, acá "result" nunca se inicializa
	// con make(..., 0, ...) — si no hay filas, queda nil (no un slice vacío).
	if msgs != nil {
		t.Errorf("FindByConversation: got %v, want nil", msgs)
	}
}

func TestMessageRepository_FindByConversation_ErrorDeScan_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)

	// NULL en una columna no nullable (string) rompe el Scan.
	mock.ExpectQuery(`SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at`).
		WithArgs("conv-1", 50, 0).
		WillReturnRows(sqlmock.NewRows(msgColumns).AddRow(nil, "conv-1", "seeker-1", "TEXT", "Hola", []byte("{}"), time.Now()))

	_, err := repo.FindByConversation(context.Background(), "conv-1", 50, 0)
	assertInternalError(t, err)
}

func TestMessageRepository_FindByConversation_MetadataMalformada_NoRompeYQuedaVacia(t *testing.T) {
	// El repo ignora deliberadamente el error de json.Unmarshal ("_ = json.Unmarshal(...)") —
	// una metadata corrupta en la fila no debe tumbar toda la consulta.
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)
	createdAt := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at`).
		WithArgs("conv-1", 50, 0).
		WillReturnRows(sqlmock.NewRows(msgColumns).AddRow("msg-1", "conv-1", "seeker-1", "TEXT", "Hola", []byte("esto no es json"), createdAt))

	msgs, err := repo.FindByConversation(context.Background(), "conv-1", 50, 0)

	if err != nil {
		t.Fatalf("FindByConversation: error inesperado: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("FindByConversation: got %d mensajes, want 1", len(msgs))
	}
	if msgs[0].ID() != "msg-1" || !msgs[0].CreatedAt().Equal(createdAt) {
		t.Errorf("mensaje: got %+v", msgs[0])
	}
	if len(msgs[0].Metadata()) != 0 {
		t.Errorf("Metadata: got %v, want vacía (el JSON corrupto se descarta silenciosamente)", msgs[0].Metadata())
	}
}

func TestMessageRepository_FindByConversation_HappyPath_MultiplesMensajes(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewMessageRepository(db)
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at`).
		WithArgs("conv-1", 10, 5).
		WillReturnRows(sqlmock.NewRows(msgColumns).
			AddRow("msg-1", "conv-1", "seeker-1", "TEXT", "Hola", []byte(`{}`), t1).
			AddRow("msg-2", "conv-1", "advertiser-1", "TEXT", "Sí, disponible", []byte(`{"foo":"bar"}`), t2))

	msgs, err := repo.FindByConversation(context.Background(), "conv-1", 10, 5)

	if err != nil {
		t.Fatalf("FindByConversation: error inesperado: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("FindByConversation: got %d mensajes, want 2", len(msgs))
	}
	if msgs[0].ID() != "msg-1" || msgs[1].ID() != "msg-2" {
		t.Errorf("orden: got [%q, %q], want [msg-1, msg-2]", msgs[0].ID(), msgs[1].ID())
	}
	if msgs[1].Metadata()["foo"] != "bar" {
		t.Errorf("Metadata: got %v", msgs[1].Metadata())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

// ─── VisitProposalRepository.Save ───────────────────────────────────────────

func TestVisitProposalRepository_Save_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	vp := mustProposal(t)

	mock.ExpectExec(`INSERT INTO visit_proposals`).
		WithArgs(vp.ID(), "conv-1", "lead-1", vp.ProposedAt(), string(domain.VisitProposalPending), nil, vp.CreatedAt()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Save(context.Background(), vp); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
}

func TestVisitProposalRepository_Save_ConResolvedAt(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	vp := mustProposal(t)
	if err := vp.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	mock.ExpectExec(`INSERT INTO visit_proposals`).
		WithArgs(vp.ID(), "conv-1", "lead-1", vp.ProposedAt(), string(domain.VisitProposalAccepted), *vp.ResolvedAt(), vp.CreatedAt()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Save(context.Background(), vp); err != nil {
		t.Fatalf("Save: error inesperado: %v", err)
	}
}

func TestVisitProposalRepository_Save_ErrorDeEjecucion_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	vp := mustProposal(t)

	mock.ExpectExec(`INSERT INTO visit_proposals`).WillReturnError(errors.New("fallo de escritura"))

	err := repo.Save(context.Background(), vp)
	assertInternalError(t, err)
}

// ─── VisitProposalRepository.FindByID ───────────────────────────────────────

var proposalColumns = []string{"id", "conversation_id", "lead_id", "proposed_at", "status", "resolved_at", "created_at"}

func TestVisitProposalRepository_FindByID_NoExiste_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)

	mock.ExpectQuery(`SELECT id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at`).
		WithArgs("no-existe").
		WillReturnError(sql.ErrNoRows)

	vp, err := repo.FindByID(context.Background(), "no-existe")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if vp != nil {
		t.Errorf("FindByID: got %+v, want nil", vp)
	}
}

func TestVisitProposalRepository_FindByID_ErrorDeScan_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)

	mock.ExpectQuery(`SELECT id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at`).
		WithArgs("vp-1").
		WillReturnError(errors.New("timeout de base de datos"))

	_, err := repo.FindByID(context.Background(), "vp-1")
	assertInternalError(t, err)
}

func TestVisitProposalRepository_FindByID_HappyPath_ConResolvedAtNull(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	proposedAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at`).
		WithArgs("vp-1").
		WillReturnRows(sqlmock.NewRows(proposalColumns).AddRow("vp-1", "conv-1", "lead-1", proposedAt, "PENDING_APPROVAL", nil, createdAt))

	vp, err := repo.FindByID(context.Background(), "vp-1")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if vp.ID() != "vp-1" || vp.Status() != domain.VisitProposalPending {
		t.Errorf("got ID=%q Status=%q", vp.ID(), vp.Status())
	}
	if vp.ResolvedAt() != nil {
		t.Errorf("ResolvedAt: got %v, want nil", vp.ResolvedAt())
	}
	if !vp.ProposedAt().Equal(proposedAt) || !vp.CreatedAt().Equal(createdAt) {
		t.Errorf("timestamps: got ProposedAt=%v CreatedAt=%v", vp.ProposedAt(), vp.CreatedAt())
	}
}

func TestVisitProposalRepository_FindByID_HappyPath_ConResolvedAtPresente(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	proposedAt := time.Now().Add(24 * time.Hour)
	resolvedAt := time.Now()
	createdAt := time.Now().Add(-time.Hour)

	mock.ExpectQuery(`SELECT id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at`).
		WithArgs("vp-1").
		WillReturnRows(sqlmock.NewRows(proposalColumns).AddRow("vp-1", "conv-1", "lead-1", proposedAt, "ACCEPTED", resolvedAt, createdAt))

	vp, err := repo.FindByID(context.Background(), "vp-1")

	if err != nil {
		t.Fatalf("FindByID: error inesperado: %v", err)
	}
	if vp.ResolvedAt() == nil || !vp.ResolvedAt().Equal(resolvedAt) {
		t.Errorf("ResolvedAt: got %v, want %v", vp.ResolvedAt(), resolvedAt)
	}
	if vp.Status() != domain.VisitProposalAccepted {
		t.Errorf("Status: got %q, want %q", vp.Status(), domain.VisitProposalAccepted)
	}
}

// ─── VisitProposalRepository.Update ──────────────────────────────────────────

func TestVisitProposalRepository_Update_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	vp := mustProposal(t)
	if err := vp.Reject(); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	mock.ExpectExec(`UPDATE visit_proposals SET status`).
		WithArgs(string(domain.VisitProposalRejected), *vp.ResolvedAt(), vp.ID()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), vp); err != nil {
		t.Fatalf("Update: error inesperado: %v", err)
	}
}

func TestVisitProposalRepository_Update_ErrorDeEjecucion_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewVisitProposalRepository(db)
	vp := mustProposal(t)

	mock.ExpectExec(`UPDATE visit_proposals SET status`).WillReturnError(errors.New("fallo de escritura"))

	err := repo.Update(context.Background(), vp)
	assertInternalError(t, err)
}

// ─── OutboxRepository.SaveTx ────────────────────────────────────────────────

func TestOutboxRepository_SaveTx_TxNoEsSqlTx_RetornaInternal(t *testing.T) {
	db, _ := newMockDB(t)
	repo := postgres.NewOutboxRepository(db)

	err := repo.SaveTx(context.Background(), "esto-no-es-una-tx", "chat.message.sent", []byte(`{}`))
	assertInternalError(t, err)
}

func TestOutboxRepository_SaveTx_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewOutboxRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO chat_outbox_events`).
		WithArgs("chat.message.sent", []byte(`{"foo":"bar"}`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	if err := repo.SaveTx(context.Background(), tx, "chat.message.sent", []byte(`{"foo":"bar"}`)); err != nil {
		t.Fatalf("SaveTx: error inesperado: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations no cumplidas: %v", err)
	}
}

func TestOutboxRepository_SaveTx_ErrorDeEjecucion_RetornaInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewOutboxRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO chat_outbox_events`).WillReturnError(errors.New("fallo de escritura"))
	mock.ExpectRollback()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	err = repo.SaveTx(context.Background(), tx, "chat.message.sent", []byte(`{}`))
	assertInternalError(t, err)
}

// ─── OutboxWorker ────────────────────────────────────────────────────────────

type fakePublisher struct{}

func (f *fakePublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	return nil
}

func TestNewOutboxWorker_NoPanic(t *testing.T) {
	db, _ := newMockDB(t)
	worker := postgres.NewOutboxWorker(db, &fakePublisher{})

	if worker == nil {
		t.Fatal("NewOutboxWorker: no debería devolver nil")
	}
	// Start() es un stub sin implementación real todavía (ver comentario en el código
	// fuente) — solo confirmamos que no panickea al invocarlo.
	worker.Start(context.Background(), nil)
}
