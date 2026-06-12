package application

import (
	"context"

	"inmo.platform/contexts/catalog/internal/domain"
	"inmo.platform/contexts/catalog/internal/ports"
)

type PropertyDTO struct {
	ID            string      `json:"id"`
	OwnerID       string      `json:"owner_id"`
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	Price         PriceDTO    `json:"price"`
	Location      LocationDTO `json:"location"`
	State         string      `json:"state"`
	OperationType string      `json:"operation_type"`
	PetPolicy     string      `json:"pet_policy"`
	DistanceM     *float64    `json:"distance_m,omitempty"`
}

type PriceDTO struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type LocationDTO struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address"`
}

type ListResponse struct {
	Properties []PropertyDTO `json:"properties"`
	Total      int           `json:"total"`
}

type ListPropertiesUseCase struct {
	repo ports.PropertyRepository
}

func NewListPropertiesUseCase(repo ports.PropertyRepository) *ListPropertiesUseCase {
	return &ListPropertiesUseCase{repo: repo}
}

func (uc *ListPropertiesUseCase) Execute(ctx context.Context, filters ports.ListFilters) (ListResponse, error) {
	properties, total, err := uc.repo.FindAll(ctx, filters)
	if err != nil {
		return ListResponse{}, err
	}

	dtos := make([]PropertyDTO, 0, len(properties))
	for _, r := range properties {
		dtos = append(dtos, toPropertyDTO(r.Property))
	}
	return ListResponse{Properties: dtos, Total: total}, nil
}

func toPropertyDTO(p *domain.Property) PropertyDTO {
	return PropertyDTO{
		ID:          p.ID(),
		OwnerID:     p.OwnerID(),
		Title:       p.Title(),
		Description: p.Description(),
		Price: PriceDTO{
			Amount:   p.Price().Amount(),
			Currency: string(p.Price().Currency()),
		},
		Location: LocationDTO{
			Latitude:  p.Location().Latitude(),
			Longitude: p.Location().Longitude(),
			Address:   p.Location().Address(),
		},
		State:         string(p.State()),
		OperationType: string(p.OperationType()),
		PetPolicy:     string(p.PetPolicy()),
	}
}
