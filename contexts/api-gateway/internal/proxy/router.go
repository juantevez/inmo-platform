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
	catalogProxy     := newReverseProxy(gr.cfg.CatalogURL)
	crmProxy         := newReverseProxy(gr.cfg.CRMURL)
	authProxy        := newReverseProxy(gr.cfg.AuthURL)
	maintenanceProxy := newReverseProxy(gr.cfg.MaintenanceURL)
	financesProxy    := newReverseProxy(gr.cfg.FinancesURL)
	contractsProxy   := newReverseProxy(gr.cfg.ContractsURL)

	authMiddleware := middleware.AuthValidator(gr.cfg.JWTSecret)

	// ── Rutas públicas ──────────────────────────────────────────────────────

	// Auth: login, registro, verificación
	gr.mux.HandleFunc("POST /api/v1/auth/login",    authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/register", authProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/auth/verify",    authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/sso/",     authProxy.ServeHTTP)

	// Catálogo (solo lectura pública)
	gr.mux.HandleFunc("GET /api/v1/properties",  catalogProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/properties/", catalogProxy.ServeHTTP)

	// ── Rutas privadas (requieren token) ────────────────────────────────────
	privateMux := http.NewServeMux()

	// Catálogo: mutaciones (publicar, reservar, cambiar estado)
	privateMux.HandleFunc("/api/v1/properties",  catalogProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/properties/", catalogProxy.ServeHTTP)

	// CRM / Leads
	privateMux.HandleFunc("/api/v1/leads",  crmProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/leads/", crmProxy.ServeHTTP)

	// Mantenimiento, Finanzas, Contratos
	privateMux.HandleFunc("/api/v1/tickets/",   maintenanceProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/tickets",    maintenanceProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/finances/",  financesProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/finances",   financesProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/contracts/", contractsProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/contracts",  contractsProxy.ServeHTTP)

	// ── Acoplamiento: cualquier ruta privada pasa por auth ──────────────────
	gr.mux.Handle("/api/v1/properties",  authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/properties/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/leads/",      authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/leads",       authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/tickets/",    authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/finances/",   authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/contracts/",  authMiddleware(privateMux))

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
