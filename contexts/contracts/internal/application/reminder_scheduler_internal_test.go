package application

// Test de caja blanca (mismo package, no "application_test"): chatURL e
// interval son detalles internos sin getters, necesarios para confirmar el
// fallback de las variables de entorno GATEWAY_URL y REMINDER_POLL_INTERVAL.

import (
	"testing"
	"time"
)

func TestNewReminderScheduler_SinEnvVars_UsaLosDefaults(t *testing.T) {
	t.Setenv("GATEWAY_URL", "")
	t.Setenv("REMINDER_POLL_INTERVAL", "")

	rs := NewReminderScheduler(nil)

	if rs.chatURL != "http://localhost:8000" {
		t.Fatalf("chatURL: got %q, want %q", rs.chatURL, "http://localhost:8000")
	}
	if rs.interval != time.Hour {
		t.Fatalf("interval: got %v, want %v", rs.interval, time.Hour)
	}
}

func TestNewReminderScheduler_ConEnvVars_LasUsaTalCual(t *testing.T) {
	t.Setenv("GATEWAY_URL", "http://gateway-interno:9000")
	t.Setenv("REMINDER_POLL_INTERVAL", "5m")

	rs := NewReminderScheduler(nil)

	if rs.chatURL != "http://gateway-interno:9000" {
		t.Fatalf("chatURL: got %q, want %q", rs.chatURL, "http://gateway-interno:9000")
	}
	if rs.interval != 5*time.Minute {
		t.Fatalf("interval: got %v, want %v", rs.interval, 5*time.Minute)
	}
}

func TestNewReminderScheduler_IntervaloInvalido_CaeAlDefault(t *testing.T) {
	t.Setenv("REMINDER_POLL_INTERVAL", "esto-no-es-una-duracion")

	rs := NewReminderScheduler(nil)

	if rs.interval != time.Hour {
		t.Fatalf("interval: got %v, want %v (default ante un valor inválido)", rs.interval, time.Hour)
	}
}
