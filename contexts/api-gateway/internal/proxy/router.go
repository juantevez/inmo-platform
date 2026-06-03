package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

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
	catalogProxy := newReverseProxy(gr.cfg.CatalogURL)
	crmProxy := newReverseProxy(gr.cfg.CRMURL)
	authProxy := newReverseProxy(gr.cfg.AuthURL)
	maintenanceProxy := newReverseProxy(gr.cfg.MaintenanceURL)
	financesProxy := newReverseProxy(gr.cfg.FinancesURL)
	contractsProxy := newReverseProxy(gr.cfg.ContractsURL)
	chatProxy := newReverseProxy(gr.cfg.ChatURL)
	chatWSProxy := newWebSocketProxy(gr.cfg.ChatURL)
	authMiddleware := middleware.AuthValidator(gr.cfg.JWTSecret)

	// ── Rutas públicas ──────────────────────────────────────────────────────

	// Auth
	gr.mux.HandleFunc("POST /api/v1/auth/login", authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/register", authProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/auth/verify", authProxy.ServeHTTP)
	gr.mux.HandleFunc("POST /api/v1/auth/sso/", authProxy.ServeHTTP)

	// Catálogo (solo lectura pública)
	gr.mux.HandleFunc("GET /api/v1/properties", catalogProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/properties/", catalogProxy.ServeHTTP)
	gr.mux.HandleFunc("GET /api/v1/properties/{id}/media", catalogProxy.ServeHTTP)

	// ── WebSocket: token via query param (?token=<jwt>) ─────────────────────
	gr.mux.Handle("/ws/chats/", authMiddleware(chatWSProxy))

	// ── Rutas privadas (requieren Bearer token) ─────────────────────────────
	privateMux := http.NewServeMux()

	// Catálogo: escritura privada
	privateMux.HandleFunc("/api/v1/properties", catalogProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/properties/", catalogProxy.ServeHTTP)

	// Perfiles de negocio
	privateMux.HandleFunc("/api/v1/catalog/profiles", catalogProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/catalog/profiles/", catalogProxy.ServeHTTP)

	// CRM / Leads
	privateMux.HandleFunc("/api/v1/leads", crmProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/leads/", crmProxy.ServeHTTP)

	// Mantenimiento, Finanzas, Contratos
	privateMux.HandleFunc("/api/v1/tickets", maintenanceProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/tickets/", maintenanceProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/finances", financesProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/finances/", financesProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/contracts", contractsProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/contracts/", contractsProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/reservations", contractsProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/reservations/", contractsProxy.ServeHTTP)

	// Chat: conversaciones, mensajes y propuestas de visita
	privateMux.HandleFunc("/api/v1/chats", chatProxy.ServeHTTP)
	privateMux.HandleFunc("/api/v1/chats/", chatProxy.ServeHTTP)

	// Disponibilidad de visitas (GET+PUT) → chat service
	// Patrón más específico: Go 1.22 lo prioriza sobre /api/v1/properties/
	privateMux.HandleFunc("/api/v1/properties/{id}/availability", chatProxy.ServeHTTP)

	// ── Acoplamiento auth → privateMux ──────────────────────────────────────
	gr.mux.Handle("/api/v1/properties", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/properties/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/catalog/profiles", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/catalog/profiles/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/leads", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/leads/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/tickets/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/finances/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/contracts/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/reservations", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/reservations/", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/chats", authMiddleware(privateMux))
	gr.mux.Handle("/api/v1/chats/", authMiddleware(privateMux))

	return gr.mux
}

// ── HTTP reverse proxy estándar ─────────────────────────────────────────────

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

// ── WebSocket proxy (tunnel TCP full-duplex) ────────────────────────────────
//
// httputil.ReverseProxy no hace el handshake de upgrade WebSocket correctamente:
// no copia el header "Connection: Upgrade" ni hace hijack del TCP.
// Esta implementación abre una conexión TCP directa al backend y copia bytes
// en ambas direcciones, lo que soporta cualquier protocolo sobre HTTP upgrade.

func newWebSocketProxy(target string) http.Handler {
	urlTarget, _ := url.Parse(target)
	backendHost := urlTarget.Host // ej: "127.0.0.1:8086"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Si no es un upgrade WebSocket, caemos al proxy HTTP normal
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			newReverseProxy(target).ServeHTTP(w, r)
			return
		}

		// 1. Hijack la conexión TCP del cliente
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "WebSocket not supported by this server", http.StatusInternalServerError)
			return
		}
		clientConn, clientBuf, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, "Hijack failed", http.StatusInternalServerError)
			return
		}
		defer clientConn.Close()

		// 2. Conectar al backend por TCP
		backendConn, err := net.Dial("tcp", backendHost)
		if err != nil {
			fmt.Fprintf(clientConn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
			return
		}
		defer backendConn.Close()

		// 3. Reenviar el request HTTP original (con los headers de upgrade) al backend
		if err := r.Write(backendConn); err != nil {
			fmt.Fprintf(clientConn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
			return
		}

		// 4. Copiar en ambas direcciones de forma concurrente (full-duplex)
		done := make(chan struct{}, 2)

		go func() {
			// backend → cliente (incluye el 101 Switching Protocols)
			io.Copy(clientConn, backendConn) //nolint:errcheck
			done <- struct{}{}
		}()

		go func() {
			// cliente → backend (frames WebSocket)
			// clientBuf puede tener bytes en el buffer del hijack
			if clientBuf.Reader.Buffered() > 0 {
				io.Copy(backendConn, clientBuf.Reader) //nolint:errcheck
			} else {
				io.Copy(backendConn, clientConn) //nolint:errcheck
			}
			done <- struct{}{}
		}()

		// Esperar a que cualquiera de los dos lados cierre
		<-done
	})
}
