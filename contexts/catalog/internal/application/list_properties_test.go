package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/catalog/internal/application"
	"inmo.platform/contexts/catalog/internal/ports"
)

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestListProperties_ErrorDelRepositorio_RetornaErrorSinEnvolver(t *testing.T) {
	boom := errors.New("timeout de base de datos")
	repo := &fakePropertyRepo{
		findAllFn: func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
			return nil, 0, boom
		},
	}
	uc := application.NewListPropertiesUseCase(repo)

	_, err := uc.Execute(context.Background(), ports.ListFilters{})

	if !errors.Is(err, boom) {
		t.Fatalf("Execute: got %v, want %v", err, boom)
	}
}

func TestListProperties_SinResultados_RetornaListaVacia(t *testing.T) {
	repo := &fakePropertyRepo{
		findAllFn: func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
			return nil, 0, nil
		},
	}
	uc := application.NewListPropertiesUseCase(repo)

	resp, err := uc.Execute(context.Background(), ports.ListFilters{})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.Properties == nil {
		t.Error("Properties: no debería ser nil (se inicializa con make([]PropertyDTO, 0, ...))")
	}
	if len(resp.Properties) != 0 || resp.Total != 0 {
		t.Errorf("Execute: got %+v, want lista vacía", resp)
	}
}

func TestListProperties_PropagaLosFiltrosSinModificar(t *testing.T) {
	var captured ports.ListFilters
	want := ports.ListFilters{
		State: "AVAILABLE", OperationType: "SALE", PetPolicy: "ALLOWED", OwnerID: "owner-1",
		MinPrice: 100, MaxPrice: 500, Limit: 10, Offset: 5,
		Latitude: -34.6, Longitude: -58.4, RadiusKm: 5,
	}
	repo := &fakePropertyRepo{
		findAllFn: func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
			captured = filters
			return nil, 0, nil
		},
	}
	uc := application.NewListPropertiesUseCase(repo)

	if _, err := uc.Execute(context.Background(), want); err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if captured != want {
		t.Errorf("filtros propagados: got %+v, want %+v", captured, want)
	}
}

func TestListProperties_HappyPath_MapeaTodosLosCamposDelDTO(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	distance := 3.5
	repo := &fakePropertyRepo{
		findAllFn: func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
			return []ports.PropertyResult{
				{Property: prop, DistanceM: &distance},
			}, 1, nil
		},
	}
	uc := application.NewListPropertiesUseCase(repo)

	resp, err := uc.Execute(context.Background(), ports.ListFilters{})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.Total != 1 || len(resp.Properties) != 1 {
		t.Fatalf("Execute: got %+v, want 1 propiedad", resp)
	}
	dto := resp.Properties[0]
	if dto.ID != prop.ID() || dto.OwnerID != prop.OwnerID() || dto.Title != prop.Title() || dto.Description != prop.Description() {
		t.Errorf("PropertyDTO campos básicos: got %+v", dto)
	}
	if dto.Price.Amount != prop.Price().Amount() || dto.Price.Currency != string(prop.Price().Currency()) {
		t.Errorf("PropertyDTO.Price: got %+v", dto.Price)
	}
	if dto.Location.Latitude != prop.Location().Latitude() || dto.Location.Longitude != prop.Location().Longitude() ||
		dto.Location.Address != prop.Location().Address() {
		t.Errorf("PropertyDTO.Location: got %+v", dto.Location)
	}
	if dto.State != string(prop.State()) || dto.OperationType != string(prop.OperationType()) || dto.PetPolicy != string(prop.PetPolicy()) {
		t.Errorf("PropertyDTO estado/tipo/mascotas: got State=%q OperationType=%q PetPolicy=%q", dto.State, dto.OperationType, dto.PetPolicy)
	}
	if dto.DistanceM == nil || *dto.DistanceM != distance {
		t.Errorf("PropertyDTO.DistanceM: got %v, want %v", dto.DistanceM, distance)
	}
}

func TestListProperties_SinBusquedaGeoespacial_DistanceMEsNil(t *testing.T) {
	prop := buildProperty(t, "prop-1", "owner-1")
	repo := &fakePropertyRepo{
		findAllFn: func(ctx context.Context, filters ports.ListFilters) ([]ports.PropertyResult, int, error) {
			return []ports.PropertyResult{{Property: prop, DistanceM: nil}}, 1, nil
		},
	}
	uc := application.NewListPropertiesUseCase(repo)

	resp, err := uc.Execute(context.Background(), ports.ListFilters{})

	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}
	if resp.Properties[0].DistanceM != nil {
		t.Errorf("DistanceM: got %v, want nil cuando la consulta no fue geoespacial", resp.Properties[0].DistanceM)
	}
}
