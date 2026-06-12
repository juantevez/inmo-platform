package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"inmo.platform/contexts/contracts/internal/ports"
)

// ReminderScheduler escanea reservas confirmadas y envía un recordatorio
// al inquilino exactamente 24hs antes del check-in.
//
// Diseño:
//   - Corre en su propia goroutine, lanzada desde main.go.
//   - Escanea cada hora (configurable vía REMINDER_POLL_INTERVAL).
//   - Marca la reserva como "reminder_sent" para no reenviar (campo en BD, ver migration).
//   - Publica un mensaje de sistema en el chat vía Gateway.

type ReminderScheduler struct {
	resRepo  ports.ReservationRepository
	chatURL  string
	client   *http.Client
	interval time.Duration
}

func NewReminderScheduler(resRepo ports.ReservationRepository) *ReminderScheduler {
	chatURL := os.Getenv("GATEWAY_URL")
	if chatURL == "" {
		chatURL = "http://localhost:8000"
	}

	interval := time.Hour
	if v := os.Getenv("REMINDER_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	return &ReminderScheduler{
		resRepo:  resRepo,
		chatURL:  chatURL,
		client:   &http.Client{Timeout: 8 * time.Second},
		interval: interval,
	}
}

// Start bloquea hasta que ctx sea cancelado.
func (rs *ReminderScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	log.Printf("[REMINDER] Scheduler iniciado. Escaneando cada %v...\n", rs.interval)

	// Ejecutar inmediatamente al arrancar, luego en cada tick
	rs.scan(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("[REMINDER] Scheduler detenido.")
			return
		case <-ticker.C:
			rs.scan(ctx)
		}
	}
}

func (rs *ReminderScheduler) scan(ctx context.Context) {
	// Ventana: reservas con check-in entre ahora+23h y ahora+25h → zona de "mañana"
	from := time.Now().UTC().Add(23 * time.Hour)
	to := time.Now().UTC().Add(25 * time.Hour)

	reservations, err := rs.resRepo.FindConfirmedCheckingInBetween(ctx, from, to)
	if err != nil {
		log.Printf("[REMINDER ERROR] Error al buscar reservas próximas: %v\n", err)
		return
	}

	if len(reservations) == 0 {
		return
	}

	log.Printf("[REMINDER] %d reserva(s) con check-in en ~24hs\n", len(reservations))

	for _, r := range reservations {
		if err := rs.sendReminder(ctx, r.ID(), r.PropertyID(), r.TenantID(), r.OwnerID(), r.CheckInDate().Format("2006-01-02"), r.CheckInDate()); err != nil {
			log.Printf("[REMINDER ERROR] Reserva %s: %v\n", r.ID(), err)
			continue
		}

		// Marcar como enviado para no reenviar en el próximo scan
		if err := rs.resRepo.MarkReminderSent(ctx, r.ID()); err != nil {
			log.Printf("[REMINDER ERROR] No se pudo marcar reminder_sent para %s: %v\n", r.ID(), err)
		}
	}
}

func (rs *ReminderScheduler) sendReminder(
	ctx context.Context,
	reservationID, propertyID, tenantID, ownerID, checkInDate string,
	checkInTime time.Time,
) error {
	text := fmt.Sprintf(
		"⏰ *Recordatorio: tu check-in es mañana.*\n\n"+
			"📅 Fecha: %s\n"+
			"🏠 Reserva ID: %s\n\n"+
			"Podés coordinar los detalles de llegada con el propietario desde este chat.",
		checkInDate, reservationID,
	)

	// 1. Resolver el chat
	chatID, err := rs.resolveChat(ctx, propertyID, tenantID, ownerID)
	if err != nil {
		return fmt.Errorf("no se pudo resolver chat: %w", err)
	}

	// 2. Postear mensaje de sistema
	body, _ := json.Marshal(map[string]interface{}{
		"body":         text,
		"message_type": "system",
	})

	url := fmt.Sprintf("%s/api/v1/chats/%s/messages", rs.chatURL, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", os.Getenv("SERVICE_TOKEN"))
	req.Header.Set("X-User-Id", "system")

	resp, err := rs.client.Do(req)
	if err != nil {
		return fmt.Errorf("error HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("chat service respondió %d", resp.StatusCode)
	}

	log.Printf("[REMINDER] Recordatorio enviado — reserva %s, chat %s\n", reservationID, chatID)
	return nil
}

func (rs *ReminderScheduler) resolveChat(ctx context.Context, propertyID, tenantID, ownerID string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"property_id":   propertyID,
		"advertiser_id": ownerID,
	})

	url := fmt.Sprintf("%s/api/v1/chats", rs.chatURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", os.Getenv("SERVICE_TOKEN"))
	req.Header.Set("X-User-Id", tenantID)

	resp, err := rs.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat service respondió %d", resp.StatusCode)
	}

	var result struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	chatID := result.ID
	if chatID == "" {
		chatID = result.ConversationID
	}
	if chatID == "" {
		return "", fmt.Errorf("chat service no devolvió ID")
	}
	return chatID, nil
}
