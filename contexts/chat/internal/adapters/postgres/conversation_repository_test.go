package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"inmo.platform/contexts/chat/internal/adapters/postgres"
	"inmo.platform/contexts/chat/internal/domain"
)

var convColumns = []string{
	"id", "property_id", "property_title",
	"seeker_id", "seeker_name",
	"advertiser_id", "advertiser_name", "lead_id",
	"created_at", "updated_at",
}

var convSummaryColumns = append(append([]string{}, convColumns...), "last_message")

func TestConversationRepository_Save_InsertaConExito(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	conv, err := domain.NewConversation("prop-1", "Depto Centro", "seeker-1", "Juan", "adv-1", "María")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}

	mock.ExpectExec(`INSERT INTO conversations`).
		WithArgs(conv.ID(), conv.PropertyID(), conv.PropertyTitle(),
			conv.SeekerID(), conv.SeekerName(),
			conv.AdvertiserID(), conv.AdvertiserName(),
			conv.LeadID(), conv.CreatedAt(), conv.UpdatedAt()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Save(context.Background(), conv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestConversationRepository_Save_ErrorDeDB_RetornaAppErrInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	conv, err := domain.NewConversation("prop-1", "Depto Centro", "seeker-1", "Juan", "adv-1", "María")
	if err != nil {
		t.Fatalf("NewConversation: %v", err)
	}

	mock.ExpectExec(`INSERT INTO conversations`).WillReturnError(errors.New("fallo de conexión"))

	err = repo.Save(context.Background(), conv)
	assertInternalError(t, err)
}

func TestConversationRepository_FindByID_Encontrada(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("conv-1").
		WillReturnRows(sqlmock.NewRows(convColumns).
			AddRow("conv-1", "prop-1", "Depto Centro", "seeker-1", "Juan", "adv-1", "María", "lead-9", now, now))

	conv, err := repo.FindByID(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if conv == nil {
		t.Fatal("esperaba una conversación, obtuve nil")
	}
	if conv.ID() != "conv-1" || conv.LeadID() != "lead-9" {
		t.Fatalf("conversación mapeada incorrectamente: %+v", conv)
	}
}

func TestConversationRepository_FindByID_NoEncontrada_RetornaNilSinError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("conv-x").
		WillReturnRows(sqlmock.NewRows(convColumns))

	conv, err := repo.FindByID(context.Background(), "conv-x")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if conv != nil {
		t.Fatalf("esperaba nil, obtuve %+v", conv)
	}
}

func TestConversationRepository_FindByID_ErrorDeDB_RetornaAppErrInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("conv-1").
		WillReturnError(errors.New("timeout de red"))

	_, err := repo.FindByID(context.Background(), "conv-1")
	assertInternalError(t, err)
}

func TestConversationRepository_FindByParticipant_MapeaResultadosConUltimoMensaje(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(convSummaryColumns).
			AddRow("conv-1", "prop-1", "Depto Centro", "user-1", "Juan", "adv-1", "María", "", now, now, "Hola, ¿sigue disponible?").
			AddRow("conv-2", "prop-2", "Casa Norte", "seeker-2", "Ana", "user-1", "Juan", "lead-5", now, now, ""))

	result, err := repo.FindByParticipant(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("FindByParticipant: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("esperaba 2 conversaciones, obtuve %d", len(result))
	}
	if result[0].LastMessage != "Hola, ¿sigue disponible?" {
		t.Fatalf("last_message mapeado incorrectamente: %q", result[0].LastMessage)
	}
	if result[1].Conversation.LeadID() != "lead-5" {
		t.Fatalf("lead_id mapeado incorrectamente: %q", result[1].Conversation.LeadID())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestConversationRepository_FindByParticipant_SinResultados(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT`).
		WithArgs("user-sin-chats").
		WillReturnRows(sqlmock.NewRows(convSummaryColumns))

	result, err := repo.FindByParticipant(context.Background(), "user-sin-chats")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("esperaba slice vacío, obtuve %d elementos", len(result))
	}
}

func TestConversationRepository_FindByParticipant_ErrorDeQuery_RetornaAppErrInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT`).WithArgs("user-1").WillReturnError(errors.New("conexión perdida"))

	_, err := repo.FindByParticipant(context.Background(), "user-1")
	assertInternalError(t, err)
}

func TestConversationRepository_FindByParticipant_ErrorDeScan_RetornaAppErrInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	// NULL en "id" (destino string no nullable) rompe el Scan.
	mock.ExpectQuery(`SELECT`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(convSummaryColumns).
			AddRow(nil, "prop-1", "Depto Centro", "user-1", "Juan", "adv-1", "María", "", time.Now(), time.Now(), ""))

	_, err := repo.FindByParticipant(context.Background(), "user-1")
	assertInternalError(t, err)
}

func TestConversationRepository_FindByParticipant_ErrorDeIteracionDeFilas(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	// rows.Err() se ejecuta luego de agotar el iterador — CloseError simula un
	// fallo de conexión detectado recién al terminar de leer las filas.
	mock.ExpectQuery(`SELECT`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(convSummaryColumns).
			AddRow("conv-1", "prop-1", "Depto Centro", "user-1", "Juan", "adv-1", "María", "", time.Now(), time.Now(), "").
			RowError(0, errors.New("fila corrupta")))

	_, err := repo.FindByParticipant(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperaba un error de rows.Err(), obtuve nil")
	}
}

func TestConversationRepository_FindByPropertyAndParticipants_Encontrada(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("prop-1", "seeker-1", "adv-1").
		WillReturnRows(sqlmock.NewRows(convColumns).
			AddRow("conv-1", "prop-1", "Depto Centro", "seeker-1", "Juan", "adv-1", "María", "", now, now))

	conv, err := repo.FindByPropertyAndParticipants(context.Background(), "prop-1", "seeker-1", "adv-1")
	if err != nil {
		t.Fatalf("FindByPropertyAndParticipants: %v", err)
	}
	if conv == nil || conv.ID() != "conv-1" {
		t.Fatalf("conversación mapeada incorrectamente: %+v", conv)
	}
}

func TestConversationRepository_FindByPropertyAndParticipants_NoEncontrada(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("prop-1", "seeker-1", "adv-1").
		WillReturnRows(sqlmock.NewRows(convColumns))

	conv, err := repo.FindByPropertyAndParticipants(context.Background(), "prop-1", "seeker-1", "adv-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if conv != nil {
		t.Fatalf("esperaba nil, obtuve %+v", conv)
	}
}

func TestConversationRepository_FindByPropertyAndParticipants_ErrorDeDB_RetornaAppErrInternal(t *testing.T) {
	db, mock := newMockDB(t)
	repo := postgres.NewConversationRepository(db)

	mock.ExpectQuery(`SELECT id, property_id, property_title, seeker_id, seeker_name,`).
		WithArgs("prop-1", "seeker-1", "adv-1").
		WillReturnError(errors.New("timeout de red"))

	_, err := repo.FindByPropertyAndParticipants(context.Background(), "prop-1", "seeker-1", "adv-1")
	assertInternalError(t, err)
}
