package config_test

import (
	"os"
	"testing"

	"inmo.platform/contexts/api-gateway/internal/config"
)

// allConfigEnvVars lista todas las variables que Load() consulta.
// Centralizado acá para que si se agrega una variable nueva al config,
// el compilador no te avise pero al menos el test de defaults falle rápido.
var allConfigEnvVars = []string{
	"GATEWAY_PORT",
	"CATALOG_SERVICE_URL",
	"CRM_SERVICE_URL",
	"AUTH_SERVICE_URL",
	"MAINTENANCE_SERVICE_URL",
	"FINANCES_SERVICE_URL",
	"CONTRACTS_SERVICE_URL",
	"JWT_SECRET",
}

// TestLoad_Defaults verifica que Load() devuelve los valores hardcodeados
// cuando no hay ninguna variable de entorno seteada.
func TestLoad_Defaults(t *testing.T) {
	cleanEnv(t, allConfigEnvVars...)

	cfg := config.Load()

	cases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Port", cfg.Port, ":8000"},
		{"CatalogURL", cfg.CatalogURL, "http://127.0.0.1:8081"},
		{"CRMURL", cfg.CRMURL, "http://127.0.0.1:8084"},
		{"AuthURL", cfg.AuthURL, "http://127.0.0.1:8080"},
		{"MaintenanceURL", cfg.MaintenanceURL, "http://127.0.0.1:8083"},
		{"FinancesURL", cfg.FinancesURL, "http://127.0.0.1:8082"},
		{"ContractsURL", cfg.ContractsURL, "http://127.0.0.1:8085"},
		{"JWTSecret", cfg.JWTSecret, "dev_secret_local"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expected {
				t.Errorf("got %q, want %q", tc.got, tc.expected)
			}
		})
	}
}

// TestLoad_FullOverride verifica que todas las env vars sobreescriben los defaults.
func TestLoad_FullOverride(t *testing.T) {
	t.Setenv("GATEWAY_PORT", ":9000")
	t.Setenv("CATALOG_SERVICE_URL", "http://catalog:8081")
	t.Setenv("CRM_SERVICE_URL", "http://crm:8084")
	t.Setenv("AUTH_SERVICE_URL", "http://auth:8086")
	t.Setenv("MAINTENANCE_SERVICE_URL", "http://maintenance:8085")
	t.Setenv("FINANCES_SERVICE_URL", "http://finances:8082")
	t.Setenv("CONTRACTS_SERVICE_URL", "http://contracts:8083")
	t.Setenv("JWT_SECRET", "super_secret_prod_key")

	cfg := config.Load()

	cases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Port", cfg.Port, ":9000"},
		{"CatalogURL", cfg.CatalogURL, "http://catalog:8081"},
		{"CRMURL", cfg.CRMURL, "http://crm:8084"},
		{"AuthURL", cfg.AuthURL, "http://auth:8086"},
		{"MaintenanceURL", cfg.MaintenanceURL, "http://maintenance:8085"},
		{"FinancesURL", cfg.FinancesURL, "http://finances:8082"},
		{"ContractsURL", cfg.ContractsURL, "http://contracts:8083"},
		{"JWTSecret", cfg.JWTSecret, "super_secret_prod_key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expected {
				t.Errorf("got %q, want %q", tc.got, tc.expected)
			}
		})
	}
}

// TestLoad_PartialOverride verifica que solo las variables seteadas se sobreescriben
// y el resto mantiene sus defaults.
// Cubre la rama "exists=true" de getEnv solo para JWT_SECRET,
// y la rama "exists=false" (fallback) para las demás.
func TestLoad_PartialOverride(t *testing.T) {
	cleanEnv(t, allConfigEnvVars...)

	// Solo sobreescribimos el secret
	t.Setenv("JWT_SECRET", "only_this_one")

	cfg := config.Load()

	if cfg.JWTSecret != "only_this_one" {
		t.Errorf("JWTSecret: got %q, want %q", cfg.JWTSecret, "only_this_one")
	}

	// El resto debe seguir siendo default
	defaults := map[string]string{
		"Port":           cfg.Port,
		"CatalogURL":     cfg.CatalogURL,
		"CRMURL":         cfg.CRMURL,
		"AuthURL":        cfg.AuthURL,
		"MaintenanceURL": cfg.MaintenanceURL,
		"FinancesURL":    cfg.FinancesURL,
		"ContractsURL":   cfg.ContractsURL,
	}
	expected := map[string]string{
		"Port":           ":8000",
		"CatalogURL":     "http://127.0.0.1:8081",
		"CRMURL":         "http://127.0.0.1:8084",
		"AuthURL":        "http://127.0.0.1:8080",
		"MaintenanceURL": "http://127.0.0.1:8083",
		"FinancesURL":    "http://127.0.0.1:8082",
		"ContractsURL":   "http://127.0.0.1:8085",
	}

	for field, got := range defaults {
		if want := expected[field]; got != want {
			t.Errorf("%s: got %q, want default %q", field, got, want)
		}
	}
}

// TestLoad_EmptyStringEnvVar verifica el edge case de una variable seteada con "".
// os.LookupEnv devuelve exists=true cuando la variable existe aunque el valor sea vacío,
// por lo que getEnv debe devolver "" y NO el fallback.
// Este test documenta y protege ese comportamiento explícitamente.
func TestLoad_EmptyStringEnvVar(t *testing.T) {
	cleanEnv(t, allConfigEnvVars...)

	// Seteamos JWT_SECRET a string vacío explícitamente.
	// A diferencia de "la variable no existe", esto sí cuenta como override.
	t.Setenv("JWT_SECRET", "")

	cfg := config.Load()

	if cfg.JWTSecret != "" {
		t.Errorf("JWTSecret con env var vacía: got %q, want empty string (no fallback)", cfg.JWTSecret)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// cleanEnv hace Unsetenv de cada clave y registra Cleanup para restaurar
// el valor original al terminar el test. Así no contaminamos otros tests
// que corran en paralelo o en secuencia.
func cleanEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		original, exists := os.LookupEnv(key)
		os.Unsetenv(key) // limpiamos para este test
		t.Cleanup(func() {
			if exists {
				os.Setenv(key, original)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}
