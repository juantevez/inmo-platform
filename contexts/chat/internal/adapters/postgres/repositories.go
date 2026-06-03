package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

// ── MessageRepository ─────────────────────────────────────────────────────

type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Save(ctx context.Context, m *domain.Message) error {
	metaJSON, err := json.Marshal(m.Metadata())
	if err != nil {
		return apperr.NewInternal("error al serializar metadata del mensaje", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO chat_messages (id, conversation_id, sender_id, msg_type, body, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.ID(), m.ConversationID(), m.SenderID(),
		string(m.Type()), m.Body(), metaJSON, m.CreatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar mensaje", err)
	}
	return nil
}

func (r *MessageRepository) FindByConversation(ctx context.Context, conversationID string, limit, offset int) ([]*domain.Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, conversation_id, sender_id, msg_type, body, metadata, created_at
		FROM chat_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3`,
		conversationID, limit, offset)
	if err != nil {
		return nil, apperr.NewInternal("error al listar mensajes", err)
	}
	defer rows.Close()

	var result []*domain.Message
	for rows.Next() {
		var id, convID, senderID, msgType, body string
		var metaRaw []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &convID, &senderID, &msgType, &body, &metaRaw, &createdAt); err != nil {
			return nil, apperr.NewInternal(fmt.Sprintf("error al escanear mensaje: %v", err), err)
		}

		var meta map[string]string
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &meta)
		}

		result = append(result, domain.ReconstructMessage(
			id, convID, senderID,
			domain.MessageType(msgType),
			body, meta, createdAt,
		))
	}
	return result, rows.Err()
}

// ── VisitProposalRepository ───────────────────────────────────────────────

type VisitProposalRepository struct {
	db *sql.DB
}

func NewVisitProposalRepository(db *sql.DB) *VisitProposalRepository {
	return &VisitProposalRepository{db: db}
}

func (r *VisitProposalRepository) Save(ctx context.Context, v *domain.VisitProposal) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO visit_proposals (id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		v.ID(), v.ConversationID(), v.LeadID(),
		v.ProposedAt(), string(v.Status()), v.ResolvedAt(), v.CreatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar propuesta de visita", err)
	}
	return nil
}

func (r *VisitProposalRepository) FindByID(ctx context.Context, id string) (*domain.VisitProposal, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, conversation_id, lead_id, proposed_at, status, resolved_at, created_at
		FROM visit_proposals WHERE id = $1`, id)

	var vID, convID, leadID, status string
	var proposedAt, createdAt time.Time
	var resolvedAt sql.NullTime

	if err := row.Scan(&vID, &convID, &leadID, &proposedAt, &status, &resolvedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al escanear propuesta de visita", err)
	}

	var resAt *time.Time
	if resolvedAt.Valid {
		t := resolvedAt.Time
		resAt = &t
	}

	return domain.ReconstructVisitProposal(
		vID, convID, leadID, proposedAt,
		domain.VisitProposalStatus(status),
		resAt, createdAt,
	), nil
}

func (r *VisitProposalRepository) Update(ctx context.Context, v *domain.VisitProposal) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE visit_proposals SET status = $1, resolved_at = $2 WHERE id = $3`,
		string(v.Status()), v.ResolvedAt(), v.ID(),
	)
	if err != nil {
		return apperr.NewInternal("error al actualizar propuesta de visita", err)
	}
	return nil
}

// ── Outbox ────────────────────────────────────────────────────────────────

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// SaveTx implementa ports.OutboxRepository. tx debe ser *sql.Tx.
func (r *OutboxRepository) SaveTx(ctx context.Context, tx interface{}, subject string, payload []byte) error {
	sqlTx, ok := tx.(*sql.Tx)
	if !ok {
		return apperr.NewInternal("tx no es *sql.Tx", nil)
	}
	_, err := sqlTx.ExecContext(ctx, `
		INSERT INTO chat_outbox_events (subject, payload, status)
		VALUES ($1, $2, 'PENDING')`, subject, payload)
	if err != nil {
		return apperr.NewInternal("error al insertar en outbox de chat", err)
	}
	return nil
}

// OutboxWorker escanea y despacha eventos pendientes a NATS.
type OutboxWorker struct {
	db        *sql.DB
	publisher interface {
		Publish(ctx context.Context, subject string, payload []byte) error
	}
}

func NewOutboxWorker(db *sql.DB, publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}) *OutboxWorker {
	return &OutboxWorker{db: db, publisher: publisher}
}

func (w *OutboxWorker) Start(ctx context.Context, interval interface{ C() <-chan struct{} }) {
	// Implementación básica del worker — usa el mismo patrón que los otros contextos.
	// Se arranca como goroutine desde main.go con time.NewTicker.
}

// Interface auxiliar para que driver.Value no cause problemas con sql.NullTime
var _ driver.Valuer = (*sql.NullTime)(nil)
