package application_test

import (
	"context"
	"errors"
	"testing"

	"inmo.platform/contexts/crm/internal/application"
	"inmo.platform/contexts/crm/internal/domain"
)

func TestCloseLeadUseCase_ErrorAlBuscar_SePropagaTalCual(t *testing.T) {
	dbErr := errors.New("timeout de base de datos")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, dbErr },
	}
	uc := application.NewCloseLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}

func TestCloseLeadUseCase_NoEncontrado_RetornaNotFound(t *testing.T) {
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return nil, nil },
	}
	uc := application.NewCloseLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-x")
	assertNotFound(t, err)
}

func TestCloseLeadUseCase_YaCerrado_RetornaPreconditionFailed(t *testing.T) {
	l := newLeadFixture(t)
	if err := l.Close(); err != nil {
		t.Fatalf("Close setup: %v", err)
	}
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
	}
	uc := application.NewCloseLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	assertPreconditionFailed(t, err)
}

func TestCloseLeadUseCase_Exitoso_CierraYGuardaSinEvento(t *testing.T) {
	l := contactedLeadFixture(t)
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
	uc := application.NewCloseLeadUseCase(repo)

	dto, err := uc.Execute(context.Background(), "lead-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if l.State != domain.StateClosed {
		t.Fatalf("state del agregado: got %s, want %s", l.State, domain.StateClosed)
	}
	if dto.State != string(domain.StateClosed) {
		t.Fatalf("state en el DTO: got %s", dto.State)
	}
	if savedEventName != "" || savedPayload != nil {
		t.Fatalf("cerrar un lead no debería emitir evento: eventName=%q payload=%v", savedEventName, savedPayload)
	}
}

func TestCloseLeadUseCase_ErrorAlGuardar_SePropagaTalCual(t *testing.T) {
	l := newLeadFixture(t)
	dbErr := errors.New("fallo de escritura")
	repo := &fakeLeadRepo{
		getByIDFn: func(ctx context.Context, id string) (*domain.Lead, error) { return l, nil },
		saveFn: func(ctx context.Context, lead *domain.Lead, eventName string, eventPayload []byte) error {
			return dbErr
		},
	}
	uc := application.NewCloseLeadUseCase(repo)

	_, err := uc.Execute(context.Background(), "lead-1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("esperaba que el error se propague tal cual: got %v", err)
	}
}
