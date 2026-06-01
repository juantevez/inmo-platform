package domain

import (
	"time"

	"inmo.platform/shared/pkg/apperr"
)

type MediaType string

const (
	MediaTypeImage      MediaType = "IMAGE"
	MediaTypeVideo      MediaType = "VIDEO"
	MediaTypeSocialLink MediaType = "SOCIAL_LINK"
)

// PropertyMedia representa un archivo multimedia o enlace de red social asociado a una propiedad.
type PropertyMedia struct {
	id          string
	propertyID  string
	url         string
	mediaType   MediaType
	sortOrder   int
	socialLinks map[string]string
	createdAt   time.Time
	updatedAt   time.Time
}

func NewPropertyMedia(id, propertyID, url string, mediaType MediaType, sortOrder int, socialLinks map[string]string) (*PropertyMedia, error) {
	if id == "" || propertyID == "" {
		return nil, apperr.NewBadRequest("id y property_id son obligatorios", nil)
	}
	if mediaType != MediaTypeImage && mediaType != MediaTypeVideo && mediaType != MediaTypeSocialLink {
		return nil, apperr.NewBadRequest("tipo de media inválido: debe ser IMAGE, VIDEO o SOCIAL_LINK", nil)
	}
	if (mediaType == MediaTypeImage || mediaType == MediaTypeVideo) && url == "" {
		return nil, apperr.NewBadRequest("url es obligatoria para media de tipo IMAGE o VIDEO", nil)
	}
	if mediaType == MediaTypeSocialLink && len(socialLinks) == 0 {
		return nil, apperr.NewBadRequest("social_links no puede estar vacío para tipo SOCIAL_LINK", nil)
	}
	return &PropertyMedia{
		id:          id,
		propertyID:  propertyID,
		url:         url,
		mediaType:   mediaType,
		sortOrder:   sortOrder,
		socialLinks: socialLinks,
		createdAt:   time.Now(),
		updatedAt:   time.Now(),
	}, nil
}

func ReconstructPropertyMedia(id, propertyID, url string, mediaType MediaType, sortOrder int, socialLinks map[string]string, createdAt, updatedAt time.Time) *PropertyMedia {
	return &PropertyMedia{
		id:          id,
		propertyID:  propertyID,
		url:         url,
		mediaType:   mediaType,
		sortOrder:   sortOrder,
		socialLinks: socialLinks,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}
}

func (m *PropertyMedia) ID() string                      { return m.id }
func (m *PropertyMedia) PropertyID() string              { return m.propertyID }
func (m *PropertyMedia) URL() string                     { return m.url }
func (m *PropertyMedia) Type() MediaType                 { return m.mediaType }
func (m *PropertyMedia) SortOrder() int                  { return m.sortOrder }
func (m *PropertyMedia) SocialLinks() map[string]string  { return m.socialLinks }
func (m *PropertyMedia) CreatedAt() time.Time            { return m.createdAt }
func (m *PropertyMedia) UpdatedAt() time.Time            { return m.updatedAt }
