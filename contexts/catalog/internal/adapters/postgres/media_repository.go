package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/shared/pkg/apperr"
)

type MediaRepository struct {
	db *sql.DB
}

func NewMediaRepository(db *sql.DB) *MediaRepository {
	return &MediaRepository{db: db}
}

func (r *MediaRepository) SaveMedia(ctx context.Context, m *domain.PropertyMedia) error {
	socialLinksJSON, err := marshalSocialLinks(m.SocialLinks())
	if err != nil {
		return err
	}

	query := `
		INSERT INTO property_media (id, property_id, url, type, sort_order, social_links, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			url = EXCLUDED.url,
			type = EXCLUDED.type,
			sort_order = EXCLUDED.sort_order,
			social_links = EXCLUDED.social_links,
			updated_at = EXCLUDED.updated_at;
	`
	_, err = r.db.ExecContext(ctx, query,
		m.ID(), m.PropertyID(), nullableString(m.URL()), string(m.Type()),
		m.SortOrder(), socialLinksJSON, m.CreatedAt(), m.UpdatedAt(),
	)
	if err != nil {
		return apperr.NewInternal("error al guardar el media en postgres", err)
	}
	return nil
}

func (r *MediaRepository) FindByPropertyID(ctx context.Context, propertyID string) ([]*domain.PropertyMedia, error) {
	query := `
		SELECT id, property_id, url, type, sort_order, social_links, created_at, updated_at
		FROM property_media
		WHERE property_id = $1
		ORDER BY sort_order ASC, created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, propertyID)
	if err != nil {
		return nil, apperr.NewInternal("error al listar media en postgres", err)
	}
	defer rows.Close()

	var items []*domain.PropertyMedia
	for rows.Next() {
		var (
			id, propID, mediaType string
			url                   sql.NullString
			sortOrder             int
			socialLinksRaw        []byte
			createdAt, updatedAt  time.Time
		)
		if err := rows.Scan(&id, &propID, &url, &mediaType, &sortOrder, &socialLinksRaw, &createdAt, &updatedAt); err != nil {
			return nil, apperr.NewInternal("error al escanear media en postgres", err)
		}

		socialLinks, err := unmarshalSocialLinks(socialLinksRaw)
		if err != nil {
			return nil, err
		}

		items = append(items, domain.ReconstructPropertyMedia(
			id, propID, url.String, domain.MediaType(mediaType), sortOrder, socialLinks, createdAt, updatedAt,
		))
	}

	if err := rows.Err(); err != nil {
		return nil, apperr.NewInternal("error al iterar media en postgres", err)
	}
	return items, nil
}

func (r *MediaRepository) DeleteMedia(ctx context.Context, mediaID string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM property_media WHERE id = $1", mediaID)
	if err != nil {
		return apperr.NewInternal("error al eliminar media en postgres", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperr.NewNotFound("media no encontrado", errors.New("no rows deleted"))
	}
	return nil
}

func marshalSocialLinks(links map[string]string) ([]byte, error) {
	if len(links) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(links)
	if err != nil {
		return nil, apperr.NewInternal("error al serializar social_links", err)
	}
	return b, nil
}

func unmarshalSocialLinks(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var links map[string]string
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, apperr.NewInternal("error al deserializar social_links", err)
	}
	return links, nil
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
