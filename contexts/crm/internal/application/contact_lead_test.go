package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
)

func TestContactLeadUseCase_ErrorAlBuscar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, dbErr },
	}
	uc := application.NewContactLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestContactLeadUseCase_NoEncontrado_RetornaNotFound(t *testing.T) {
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil },
	}
	uc := application.NewContactLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-x")
	assertNotFound(t, err)
}

func TestContactLeadUseCase_EstadoNoPermiteContactar_RetornaPreconditionFailed(t *testing.T) {
	l := contactedLeadFixture(t) // ya está CONTACTED, no puede volver a MarkContacted
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewContactLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	assertPreconditionFailed(t, err)
}

func TestContactLeadUseCase_Exitoso_MarcaContactadoYGuardaSinEvento(t *testing.T) {
	l := newLeadFixture(t)
	var savedEventName string
	var savedPayload []byte
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			savedEventName = eventName
			savedPayload = eventPayload
			return nil
		},
	}
	uc := application.NewContactLeadUseCase(repo)

	dto, err := uc.Execute(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if l.State != domain.StateContacted {
		t.Fatalf("state del agregado: got %s, want %s", l.State, domain.StateContacted)
	}
	if dto.State != string(domain.StateContacted) {
		t.Fatalf("state en el DTO: got %s", dto.State)
	}
	if savedEventName != "" || savedPayload != nil {
		t.Fatalf("contactar un lead no debería emitir evento: eventName=%q payload=%v", savedEventName, savedPayload)
	}
}

func TestContactLeadUseCase_ErrorAlGuardar_SePropagaTalCual(t *testing.T) {
	l := newLeadFixture(t)
	dbErr := errors.New("fallo de escritura")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			return dbErr
		},
	}
	uc := application.NewContactLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}
