package middleware

import "net/http"

// CORSHandler envuelve un http.Handler para aplicar las políticas de CORS de forma global
func CORSHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// En entorno de desarrollo podés dejarlo en "*" o configurar la URL de tu Front local (ej: http://localhost:3000)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-User-Id, X-User-Role")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// 🛑 Si el navegador envía una petición de tipo OPTIONS (Preflight), respondemos OK de inmediato sin pasar al proxy
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Si es una petición normal, continúa el flujo hacia los ruteadores y proxies correspondientes
		next.ServeHTTP(w, r)
	})
}
