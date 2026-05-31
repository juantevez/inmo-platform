package config

import "os"

type Config struct {
	Port           string
	CatalogURL     string
	AuthURL        string
	MaintenanceURL string
	FinancesURL    string
	ContractsURL   string
	JWTSecret      string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("GATEWAY_PORT", ":8000"), // 🚀 Cambiado a :8000
		CatalogURL:     getEnv("CATALOG_SERVICE_URL", "http://inmo-catalog:8081"),
		AuthURL:        getEnv("AUTH_SERVICE_URL", "http://inmo-auth-identity:8080"),
		MaintenanceURL: getEnv("MAINTENANCE_SERVICE_URL", "http://inmo-maintenance:8083"),
		FinancesURL:    getEnv("FINANCES_SERVICE_URL", "http://inmo-finances:8082"),
		ContractsURL:   getEnv("CONTRACTS_SERVICE_URL", "http://inmo-contracts:8085"),
		JWTSecret:      getEnv("JWT_SECRET", "tu_super_secreto_local"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
