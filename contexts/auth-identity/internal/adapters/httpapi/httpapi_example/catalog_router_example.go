// EJEMPLO DE USO — contexts/inmo-catalog/internal/adapters/httpapi/router.go
//
// Este archivo muestra cómo aplicar el middleware JWT y los guards de permisos
// en el módulo de catálogo. Adaptá el mismo patrón para maintenance y payments.
//
// El JWT_SECRET debe ser la misma variable de entorno que usa auth-identity.
// Nunca hardcodees el secret — usá os.Getenv("JWT_SECRET").

package httpapi_example

import (
	"net/http"
	"os"

	"inmo.platform/contexts/auth-identity/internal/adapters/httpapi/middleware"
)

func NewCatalogRouter(handler *CatalogHandler) http.Handler {
	mux := http.NewServeMux()
	jwtSecret := os.Getenv("JWT_SECRET")

	// --- Rutas PÚBLICAS (sin login) ---
	// Cualquier visitante puede buscar y ver propiedades
	mux.HandleFunc("GET /api/v1/properties", handler.HandleListProperties)
	mux.HandleFunc("GET /api/v1/properties/{id}", handler.HandleGetProperty)

	// --- Rutas PROTEGIDAS ---
	// Encadenamos: JWTMiddleware valida el token → RequirePermission valida el permiso

	// Solo PROPIETARIO y AGENTE pueden publicar
	mux.Handle("POST /api/v1/properties",
		middleware.JWTMiddleware(jwtSecret)(
			middleware.RequirePermission("property:create")(
				http.HandlerFunc(handler.HandleCreateProperty),
			),
		),
	)

	// Solo el dueño del recurso o AGENTE puede editar (la validación de ownership va en el handler)
	mux.Handle("PUT /api/v1/properties/{id}",
		middleware.JWTMiddleware(jwtSecret)(
			middleware.RequirePermission("property:update")(
				http.HandlerFunc(handler.HandleUpdateProperty),
			),
		),
	)

	// Solo AGENTE y ADMIN_INMO pueden eliminar
	mux.Handle("DELETE /api/v1/properties/{id}",
		middleware.JWTMiddleware(jwtSecret)(
			middleware.RequirePermission("property:delete")(
				http.HandlerFunc(handler.HandleDeleteProperty),
			),
		),
	)

	// Postulaciones: INTERESADO e INQUILINO pueden postularse
	mux.Handle("POST /api/v1/properties/{id}/postulations",
		middleware.JWTMiddleware(jwtSecret)(
			middleware.RequirePermission("postulation:create")(
				http.HandlerFunc(handler.HandleCreatePostulation),
			),
		),
	)

	// Ver postulaciones: AGENTE y ADMIN_INMO
	mux.Handle("GET /api/v1/properties/{id}/postulations",
		middleware.JWTMiddleware(jwtSecret)(
			middleware.RequirePermission("postulation:read")(
				http.HandlerFunc(handler.HandleListPostulations),
			),
		),
	)

	return mux
}

// --- En el handler, leer el user_id del contexto para validar ownership ---
//
// func (h *CatalogHandler) HandleUpdateProperty(w http.ResponseWriter, r *http.Request) {
//     userID := middleware.UserIDFromContext(r.Context())
//     roles  := middleware.RolesFromContext(r.Context())
//
//     // Si no es AGENTE ni ADMIN_INMO, verificar que la propiedad le pertenece
//     property, _ := h.propertyRepo.FindByID(r.Context(), propertyID)
//     if !isAgentOrAdmin(roles) && property.OwnerID() != userID {
//         http.Error(w, "No podés editar una propiedad que no es tuya", http.StatusForbidden)
//         return
//     }
//     // ... resto del handler
// }

// Stubs para que compile el ejemplo
type CatalogHandler struct{}

func (h *CatalogHandler) HandleListProperties(w http.ResponseWriter, r *http.Request)    {}
func (h *CatalogHandler) HandleGetProperty(w http.ResponseWriter, r *http.Request)       {}
func (h *CatalogHandler) HandleCreateProperty(w http.ResponseWriter, r *http.Request)    {}
func (h *CatalogHandler) HandleUpdateProperty(w http.ResponseWriter, r *http.Request)    {}
func (h *CatalogHandler) HandleDeleteProperty(w http.ResponseWriter, r *http.Request)    {}
func (h *CatalogHandler) HandleCreatePostulation(w http.ResponseWriter, r *http.Request) {}
func (h *CatalogHandler) HandleListPostulations(w http.ResponseWriter, r *http.Request)  {}
