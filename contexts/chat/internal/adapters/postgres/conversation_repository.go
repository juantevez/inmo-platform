package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) Save(ctx context.Context, c *domain.Conversation) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO conversations (id, property_id, seeker_id, advertiser_id, lead_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			lead_id    = NULLIF(EXCLUDED.lead_id, ''),
			updated_at = EXCLUDED.updated_at`,
		c.ID(), c.PropertyID(), c.SeekerID(), c.AdvertiserID(),
		c.LeadID(), c.CreatedAt(), c.UpdatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar conversación", err)
	}
	return nil
}

func (r *ConversationRepository) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, property_id, seeker_id, advertiser_id, COALESCE(lead_id,''), created_at, updated_at
		FROM conversations WHERE id = $1`, id)
	return r.scanRow(row)
}

func (r *ConversationRepository) FindByParticipant(ctx context.Context, userID string) ([]*domain.Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, property_id, seeker_id, advertiser_id, COALESCE(lead_id,''), created_at, updated_at
		FROM conversations
		WHERE seeker_id = $1 OR advertiser_id = $1
		ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, apperr.NewInternal("error al listar conversaciones", err)
	}
	defer rows.Close()

	var result []*domain.Conversation
	for rows.Next() {
		c, err := r.scanRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *ConversationRepository) FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, property_id, seeker_id, advertiser_id, COALESCE(lead_id,''), created_at, updated_at
		FROM conversations
		WHERE property_id = $1 AND seeker_id = $2 AND advertiser_id = $3
		LIMIT 1`, propertyID, seekerID, advertiserID)
	c, err := r.scanRow(row)
	if err != nil || c == nil {
		return nil, err
	}
	return c, nil
}

func (r *ConversationRepository) scanRow(row *sql.Row) (*domain.Conversation, error) {
	var id, propID, seekerID, advertiserID, leadID string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &propID, &seekerID, &advertiserID, &leadID, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al escanear conversación", err)
	}
	return domain.ReconstructConversation(id, propID, seekerID, advertiserID, leadID, createdAt, updatedAt), nil
}

func (r *ConversationRepository) scanRows(rows *sql.Rows) (*domain.Conversation, error) {
	var id, propID, seekerID, advertiserID, leadID string
	var createdAt, updatedAt time.Time
	if err := rows.Scan(&id, &propID, &seekerID, &advertiserID, &leadID, &createdAt, &updatedAt); err != nil {
		return nil, apperr.NewInternal(fmt.Sprintf("error al escanear fila de conversación: %v", err), err)
	}
	return domain.ReconstructConversation(id, propID, seekerID, advertiserID, leadID, createdAt, updatedAt), nil
}
