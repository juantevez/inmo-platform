package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// contextKey es el tipo privado para las keys del contexto HTTP de este módulo.
// Debe coincidir con las keys que usan los handlers (provider_handler.go y ticket_handler.go).
type contextKey string

const (
	CtxKeyUserID      contextKey = "user_id"
	CtxKeyRoles       contextKey = "roles"
	CtxKeyPermissions contextKey = "permissions"
)

// MapTicketRoutes registra todos los endpoints del módulo de mantenimiento.
//
// Estructura de autorización por endpoint:
//
//	POST /api/v1/tickets/report     → maintenance:create  (INQUILINO, PROPIETARIO)
//	POST /api/v1/tickets/assign     → maintenance:update  (ADMIN_INMO, AGENTE)
//	POST /api/v1/tickets/quote      → maintenance:update  (PROVEEDOR)
//	POST /api/v1/tickets/approve    → maintenance:update  (PROPIETARIO, ADMIN_INMO)
//	POST /api/v1/tickets/close      → maintenance:update  (PROVEEDOR)
//	POST /api/v1/providers          → maintenance:create  (PROVEEDOR autoregistro, ADMIN_INMO)
//	GET  /api/v1/providers/search   → maintenance:update  (ADMIN_INMO, AGENTE)
func MapTicketRoutes(mux *http.ServeMux, ticketHandler *TicketHandler, providerHandler *ProviderHandler) {
	secret := os.Getenv("JWT_SECRET")

	// -------------------------------------------------------------------------
	// TICKETS
	// -------------------------------------------------------------------------

	mux.Handle("/api/v1/tickets/report",
		chain(secret, "maintenance:create", ticketHandler.ReportTicket),
	)

	mux.Handle("/api/v1/tickets/assign",
		chain(secret, "maintenance:update", ticketHandler.AssignProvider),
	)

	mux.Handle("/api/v1/tickets/quote",
		chain(secret, "maintenance:update", ticketHandler.SubmitQuote),
	)

	mux.Handle("/api/v1/tickets/approve",
		chain(secret, "maintenance:update", ticketHandler.ApproveTicket),
	)

	mux.Handle("/api/v1/tickets/close",
		chain(secret, "maintenance:update", ticketHandler.CloseTicket),
	)

	// -------------------------------------------------------------------------
	// PROVEEDORES TÉCNICOS
	// -------------------------------------------------------------------------

	// Registro: PROVEEDOR (autoregistro) o ADMIN_INMO (registra a otro)
	mux.Handle("/api/v1/providers",
		chain(secret, "maintenance:create", providerHandler.RegisterProvider),
	)

	// Búsqueda por rubro: solo para quien va a asignar un ticket
	mux.Handle("/api/v1/providers/search",
		chain(secret, "maintenance:update", providerHandler.ListProvidersByRubro),
	)
}

// chain es un helper que encadena JWTMiddleware → RequirePermission → handler
// en una sola llamada para mantener el router limpio y sin repetición.
func chain(jwtSecret, permission string, handlerFunc http.HandlerFunc) http.Handler {
	return jwtMiddleware(jwtSecret)(
		requirePermission(permission)(
			handlerFunc,
		),
	)
}

// =========================================================================
// JWT Middleware
// =========================================================================

type maintenanceClaims struct {
	Sub         string   `json:"sub"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	jwtlib.RegisteredClaims
}

// jwtMiddleware valida la firma y expiración del Bearer token.
// Si es válido, inyecta user_id, roles y permissions en el contexto.
func jwtMiddleware(secret string) func(http.Handler) http.Handler {
	secretBytes := []byte(secret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearer(r)
			if tokenStr == "" {
				respondWithError(w, http.StatusUnauthorized, "Authorization header requerido (Bearer <token>)")
				return
			}

			claims, err := parseClaims(tokenStr, secretBytes)
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "Token inválido o expirado")
				return
			}

			ctx := context.WithValue(r.Context(), CtxKeyUserID, claims.Sub)
			ctx = context.WithValue(ctx, CtxKeyRoles, claims.Roles)
			ctx = context.WithValue(ctx, CtxKeyPermissions, claims.Permissions)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requirePermission verifica que el usuario tenga el permiso requerido.
// Se encadena siempre después de jwtMiddleware.
func requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			perms, _ := r.Context().Value(CtxKeyPermissions).([]string)
			for _, p := range perms {
				if p == permission {
					next.ServeHTTP(w, r)
					return
				}
			}
			respondWithError(w, http.StatusForbidden,
				"No tenés permiso para realizar esta acción ("+permission+")")
		})
	}
}

// =========================================================================
// Helpers internos
// =========================================================================

func extractBearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func parseClaims(tokenStr string, secret []byte) (*maintenanceClaims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &maintenanceClaims{},
		func(t *jwtlib.Token) (interface{}, error) {
			// Rechazar cualquier algoritmo que no sea HS256 — previene el ataque "alg:none"
			if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
				return nil, jwtlib.ErrSignatureInvalid
			}
			return secret, nil
		},
	)
	if err != nil || !token.Valid {
		return nil, err
	}

	claims, ok := token.Claims.(*maintenanceClaims)
	if !ok {
		return nil, jwtlib.ErrTokenInvalidClaims
	}
	return claims, nil
}
