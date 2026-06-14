package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"inmo.platform/contexts/chat/internal/domain"
	"inmo.platform/contexts/chat/internal/ports"
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
		INSERT INTO conversations
			(id, property_id, property_title, seeker_id, seeker_name, advertiser_id, advertiser_name, lead_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8,''), $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			lead_id         = NULLIF(EXCLUDED.lead_id, ''),
			property_title  = EXCLUDED.property_title,
			seeker_name     = EXCLUDED.seeker_name,
			advertiser_name = EXCLUDED.advertiser_name,
			updated_at      = EXCLUDED.updated_at`,
		c.ID(), c.PropertyID(), c.PropertyTitle(),
		c.SeekerID(), c.SeekerName(),
		c.AdvertiserID(), c.AdvertiserName(),
		c.LeadID(), c.CreatedAt(), c.UpdatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar conversación", err)
	}
	return nil
}

func (r *ConversationRepository) FindByID(ctx context.Context, id string) (*domain.Conversation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, property_id, property_title, seeker_id, seeker_name,
		       advertiser_id, advertiser_name, COALESCE(lead_id,''), created_at, updated_at
		FROM conversations WHERE id = $1`, id)
	return r.scanRow(row)
}

func (r *ConversationRepository) FindByParticipant(ctx context.Context, userID string) ([]*ports.ConversationSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id, c.property_id, c.property_title,
			c.seeker_id, c.seeker_name,
			c.advertiser_id, c.advertiser_name,
			COALESCE(c.lead_id,''),
			c.created_at, c.updated_at,
			COALESCE(lm.body, '') AS last_message
		FROM conversations c
		LEFT JOIN LATERAL (
			SELECT body FROM chat_messages
			WHERE conversation_id = c.id
			ORDER BY created_at DESC
			LIMIT 1
		) lm ON true
		WHERE c.seeker_id = $1 OR c.advertiser_id = $1
		ORDER BY c.updated_at DESC`, userID)
	if err != nil {
		return nil, apperr.NewInternal("error al listar conversaciones", err)
	}
	defer rows.Close()

	var result []*ports.ConversationSummary
	for rows.Next() {
		var (
			id, propID, propTitle                   string
			seekerID, seekerName                    string
			advertiserID, advertiserName, leadID     string
			createdAt, updatedAt                    time.Time
			lastMessage                             string
		)
		if err := rows.Scan(
			&id, &propID, &propTitle,
			&seekerID, &seekerName,
			&advertiserID, &advertiserName, &leadID,
			&createdAt, &updatedAt,
			&lastMessage,
		); err != nil {
			return nil, apperr.NewInternal(fmt.Sprintf("error al escanear conversación: %v", err), err)
		}
		conv := domain.ReconstructConversation(
			id, propID, propTitle,
			seekerID, seekerName,
			advertiserID, advertiserName,
			leadID, createdAt, updatedAt,
		)
		result = append(result, &ports.ConversationSummary{
			Conversation: conv,
			LastMessage:  lastMessage,
		})
	}
	return result, rows.Err()
}

func (r *ConversationRepository) FindByPropertyAndParticipants(ctx context.Context, propertyID, seekerID, advertiserID string) (*domain.Conversation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, property_id, property_title, seeker_id, seeker_name,
		       advertiser_id, advertiser_name, COALESCE(lead_id,''), created_at, updated_at
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
	var (
		id, propID, propTitle                string
		seekerID, seekerName                 string
		advertiserID, advertiserName, leadID  string
		createdAt, updatedAt                 time.Time
	)
	if err := row.Scan(
		&id, &propID, &propTitle,
		&seekerID, &seekerName,
		&advertiserID, &advertiserName, &leadID,
		&createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperr.NewInternal("error al escanear conversación", err)
	}
	return domain.ReconstructConversation(
		id, propID, propTitle,
		seekerID, seekerName,
		advertiserID, advertiserName,
		leadID, createdAt, updatedAt,
	), nil
}
