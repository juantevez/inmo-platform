package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"inmo.platform/shared/pkg/health"
)

func NewRouter(
	propertyHandler *PropertyHandler,
	profileHandler *ProfileHandler,
	mediaHandler *MediaHandler,
	checker *health.Checker,
) http.Handler {
	mux := http.NewServeMux()

	// ── Health checks (públicos) ──────────────────────────────────────
	mux.HandleFunc("GET /healthz/live", checker.LiveHandler)
	mux.HandleFunc("GET /healthz/ready", checker.ReadyHandler)

	// ── Propiedades — públicas ────────────────────────────────────────
	mux.HandleFunc("GET /api/v1/properties", propertyHandler.List)
	mux.HandleFunc("GET /api/v1/properties/{id}/media", mediaHandler.HandleListMedia)
	mux.HandleFunc("POST /api/v1/properties/{id}/quote", propertyHandler.Quote)

	// ── Propiedades — protegidas ──────────────────────────────────────
	// El gateway ya validó el JWT e inyectó X-User-Id, X-User-Roles, X-Permissions.
	// El catalog solo verifica que el permiso necesario esté en X-Permissions.

	mux.Handle("POST /api/v1/properties",
		requirePermission("property:create")(http.HandlerFunc(propertyHandler.Publish)),
	)
	mux.Handle("PUT /api/v1/properties/{id}",
		requirePermission("property:update")(http.HandlerFunc(propertyHandler.Update)),
	)
	mux.Handle("POST /api/v1/properties/{id}/reserve",
		requireAuth()(http.HandlerFunc(propertyHandler.Reserve)),
	)

	// ── Media — protegida ─────────────────────────────────────────────
	mux.Handle("POST /api/v1/properties/{id}/media/upload-url",
		requirePermission("property:update")(http.HandlerFunc(mediaHandler.HandleGenerateUploadURL)),
	)
	mux.Handle("POST /api/v1/properties/{id}/media",
		requirePermission("property:update")(http.HandlerFunc(mediaHandler.HandleAddMedia)),
	)
	mux.Handle("DELETE /api/v1/properties/{id}/media/{mediaID}",
		requirePermission("property:update")(http.HandlerFunc(mediaHandler.HandleDeleteMedia)),
	)

	// ── Perfiles de negocio ───────────────────────────────────────────
	mux.Handle("GET /api/v1/catalog/profiles/me",
		requireAuth()(http.HandlerFunc(profileHandler.HandleGetProfile)),
	)
	mux.Handle("POST /api/v1/catalog/profiles",
		requireAuth()(http.HandlerFunc(profileHandler.HandleCreateOrUpdate)),
	)

	return mux
}

// requirePermission verifica que X-Permissions contenga el permiso requerido.
// El gateway inyecta este header tras validar el JWT — el catalog no necesita
// parsear el token nuevamente.
func requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permsHeader := r.Header.Get("X-Permissions")
			if permsHeader == "" {
				respondErr(w, http.StatusForbidden, "Sin permisos asignados — autenticación requerida")
				return
			}
			for _, p := range strings.Split(permsHeader, ",") {
				if strings.TrimSpace(p) == permission {
					next.ServeHTTP(w, r)
					return
				}
			}
			respondErr(w, http.StatusForbidden, "No tenés permiso para realizar esta acción ("+permission+")")
		})
	}
}

// requireAuth verifica que X-User-Id esté presente — el usuario está autenticado
// pero no se exige un permiso específico.
func requireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-User-Id") == "" {
				respondErr(w, http.StatusUnauthorized, "Autenticación requerida")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func respondErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
