package main

import (
	"log"
	"net/http"

	"inmo.platform/contexts/api-gateway/internal/config"
	"inmo.platform/contexts/api-gateway/internal/middleware"
	"inmo.platform/contexts/api-gateway/internal/proxy"
)

func main() {
	// 1. Cargamos configuración
	cfg := config.Load()

	// 2. Inicializamos el ruteador del Gateway
	gatewayRouter := proxy.NewRouter(cfg)
	handler := gatewayRouter.MapRoutes()

	// 3. Envolvemos todo con el Middleware global de CORS
	finalHandler := middleware.CORSHandler(handler)

	// 4. Encendemos el motor
	log.Printf("🚀 API Gateway del mundo real escuchando de forma segura en el puerto %s...", cfg.Port)
	if err := http.ListenAndServe(cfg.Port, finalHandler); err != nil {
		log.Fatalf("Error crítico en el API Gateway: %v", err)
	}
}
