package config

import "os"

type Config struct {
	Port           string
	CatalogURL     string
	CRMURL         string
	AuthURL        string
	MaintenanceURL string
	FinancesURL    string
	ContractsURL   string
	JWTSecret      string
	ChatURL        string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("GATEWAY_PORT", ":8000"),
		CatalogURL:     getEnv("CATALOG_SERVICE_URL", "http://127.0.0.1:8081"),
		CRMURL:         getEnv("CRM_SERVICE_URL", "http://127.0.0.1:8084"),
		AuthURL:        getEnv("AUTH_SERVICE_URL", "http://127.0.0.1:8080"),
		MaintenanceURL: getEnv("MAINTENANCE_SERVICE_URL", "http://127.0.0.1:8085"),
		FinancesURL:    getEnv("FINANCES_SERVICE_URL", "http://127.0.0.1:8082"),
		ContractsURL:   getEnv("CONTRACTS_SERVICE_URL", "http://127.0.0.1:8083"),
		JWTSecret:      getEnv("JWT_SECRET", "dev_secret_local"),
		ChatURL:        getEnv("CHAT_SERVICE_URL", "http://127.0.0.1:8086"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
