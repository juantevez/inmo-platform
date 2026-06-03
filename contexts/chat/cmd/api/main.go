package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"inmo.platform/contexts/chat/internal/adapters/httpapi"
	chatNats "inmo.platform/contexts/chat/internal/adapters/nats"
	"inmo.platform/contexts/chat/internal/adapters/postgres"
	wsAdapter "inmo.platform/contexts/chat/internal/adapters/websocket"
	"inmo.platform/contexts/chat/internal/application"
	"inmo.platform/shared/pkg/eventbus"
	"inmo.platform/shared/pkg/health"
	"inmo.platform/shared/pkg/pg"
)

func main() {
	log.Println("Iniciando Bounded Context: chat...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbURL := getEnv("DATABASE_URL", "postgres://inmo_user:inmo_password@localhost:5432/inmo_catalog_db?sslmode=disable")
	natsURL := getEnv("NATS_URL", nats.DefaultURL)
	serverPort := getEnv("SERVER_PORT", ":8086")

	// 1. PostgreSQL
	dbPool, err := pg.NewPool(pg.Config{
		URL:          dbURL,
		MaxOpenConns: 10,
		MaxIdleConns: 2,
		MaxIdleTime:  5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("Chat: no se pudo conectar a Postgres: %v", err)
	}
	defer dbPool.Close()

	// 2. NATS JetStream
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Chat: error al conectar a NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Chat: error al inicializar JetStream: %v", err)
	}

	// Asegurar streams necesarios
	for _, cfg := range []jetstream.StreamConfig{
		{Name: "chat", Subjects: []string{"chat.>"}},
		{Name: "crm", Subjects: []string{"crm.>"}},
	} {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			log.Fatalf("Chat: no se pudo crear stream '%s': %v", cfg.Name, err)
		}
	}

	// 3. Repositorios
	convRepo := postgres.NewConversationRepository(dbPool)
	msgRepo := postgres.NewMessageRepository(dbPool)
	proposalRepo := postgres.NewVisitProposalRepository(dbPool)

	// 4. WebSocket Hub
	hub := wsAdapter.NewHub()

	// 5. Event Publisher
	publisher := eventbus.NewEventPublisher(js)

	// 6. Casos de uso
	startConvUC := application.NewStartConversationUseCase(convRepo)
	listConvsUC := application.NewListConversationsUseCase(convRepo)
	getMessagesUC := application.NewGetMessagesUseCase(convRepo, msgRepo)
	sendMsgUC := application.NewSendMessageUseCase(dbPool, convRepo, msgRepo, hub, publisher)
	proposeVisitUC := application.NewProposeVisitUseCase(convRepo, msgRepo, proposalRepo, hub, publisher)

	// 7. Workers y subscribers con tracking de estado
	var wg sync.WaitGroup

	outboxStatus := health.NewWorkerStatus("outbox_worker")
	crmSubStatus := health.NewWorkerStatus("crm_subscriber")

	outboxWorker := postgres.NewChatOutboxWorker(dbPool, js)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer outboxStatus.MarkStopped()
		outboxWorker.Start(ctx, 10*time.Second)
	}()

	crmSub := chatNats.NewCRMEventSubscriber(js, proposalRepo, msgRepo, convRepo, hub)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer crmSubStatus.MarkStopped()
		if err := crmSub.StartConsume(ctx); err != nil {
			log.Printf("[CHAT ERROR] Subscriber CRM: %v\n", err)
		}
	}()

	// 8. HTTP server
	checker := health.NewChecker(dbPool, nc, outboxStatus, crmSubStatus)
	handler := httpapi.NewChatHandler(startConvUC, listConvsUC, getMessagesUC, sendMsgUC, proposeVisitUC)
	router := httpapi.NewRouter(handler, hub, checker)

	server := &http.Server{
		Addr:         serverPort,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Servidor de Chat corriendo en %s\n", serverPort)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error crítico en el servidor de chat: %v", err)
		}
	case sig := <-quit:
		log.Printf("[CHAT] Señal recibida (%s), apagado graceful...", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	cancel()
	wg.Wait()
	log.Println("[CHAT] Módulo apagado correctamente.")
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
