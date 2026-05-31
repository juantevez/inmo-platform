package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"inmo.platform/contexts/api-gateway/internal/config"
	"inmo.platform/contexts/api-gateway/internal/middleware"
)

type GatewayRouter struct {
	cfg *config.Config
	mux *http.ServeMux
}

func NewRouter(cfg *config.Config) *GatewayRouter {
	return &GatewayRouter{
		cfg: cfg,
		mux: http.NewServeMux(),
	}
}

func (gr *GatewayRouter) MapRoutes() http.Handler {
	// Inicializamos los proxies destinos
	catalogProxy := newReverseProxy(gr.cfg.CatalogURL)
	authProxy := newReverseProxy(gr.cfg.AuthURL)
	maintenanceProxy := newReverseProxy(gr.cfg.MaintenanceURL)
	financesProxy := newReverseProxy(gr.cfg.FinancesURL)
	contractsProxy := newReverseProxy(gr.cfg.ContractsURL)

	// Instanciamos el middleware de autenticación
	authMiddleware := middleware.AuthValidator(gr.cfg.JWTSecret)

	// ==========================================
	// 🌍 1. RUTAS PÚBLICAS (Mux Principal)
	// ==========================================

	// Auth-Identity (Login, Registro, Verificación)
	gr.mux.HandleFunc("POST /api/v1/auth/login", authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/register", authProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/auth/verify", authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/sso/", authProxy.ServeHTTP) // Captura Google y Meta de una

	// Catalog Público (Landing Page): Solo permitimos métodos de lectura (GET)
	// En Go 1.22+, "GET /path/" actúa como un comodín que machea "GET /path" y cualquier sub-ruta (ej: /properties/PROP-001)
	gr.mux.HandleFunc("GET /api/v1/properties", catalogProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/properties/", catalogProxy.ServeHTTP)

	// ==========================================
	// 🔒 2. RUTAS PRIVADAS (Sub-Mux Protegido)
	// ==========================================
	privateMux := http.NewServeMux()

	// Acá adentro registrás todo lo que requiere token. No hace falta ponerle el método ("POST", "PUT")
	// a las rutas de los otros contextos si querés que el proxy delegue absolutamente todo.
	privateMux.HandleFunc("/api/v1/properties/", catalogProxy.ServeHTTP) // Captura el POST de publicar y el POST de reservar!
	privateMux.HandleFunc("/api/v1/properties", catalogProxy.ServeHTTP)

	privateMux.HandleFunc("/api/v1/tickets/", maintenanceProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/tickets", maintenanceProxy.ServeHTTP)

	privateMux.HandleFunc("/api/v1/finances/", financesProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/contracts/", contractsProxy.ServeHTTP)

	// ==========================================
	// 🔀 3. ACOPLAMIENTO Y FILTRADO FINAL
	// ==========================================

	// Para Maintenance, Finances y Contracts, delegamos todo el árbol de rutas al flujo privado
	gr.mux.Handle("/api/v1/tickets/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/finances/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/contracts/", authMiddleware(privateMux))

	// 🔥 TRUCO CLAVE PARA PROPIEDADES:
	// Como el ruteador nativo de Go prioriza por patrones más específicos, las peticiones "GET" van a machear
	// directo arriba (en las públicas). Cualquier otro método no-GET (como POST, PUT, DELETE) va a caer acá abajo.
	// Al usar la barra al final y sin especificar el método, obligamos a que CUALQUIER mutación a properties pase por Auth.
	gr.mux.Handle("/api/v1/properties", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/properties/", authMiddleware(privateMux))

	return gr.mux
}

func newReverseProxy(target string) *httputil.ReverseProxy {
	urlTarget, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(urlTarget)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = urlTarget.Host
	}
	return proxy
}
