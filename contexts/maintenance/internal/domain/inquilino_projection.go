package domain

import "time"

// InquilinoProjection es la copia local mínima de un inquilino en maintenance.
//
// Solo contiene los datos que maintenance necesita para operar de forma autónoma:
//   - user_id: clave de vínculo con auth_db y con los tickets que abre
//   - email: para notificaciones (ticket abierto, presupuesto aprobado, cierre)
//   - status: para validar que el inquilino puede seguir abriendo tickets
//
// Se sincroniza via el evento auth.user.created cuando role = "INQUILINO".
// Si el inquilino es suspendido, auth publica auth.user.suspended y actualizamos status.
type InquilinoProjection struct {
	userID    string
	email     string
	status    string // ACTIVE | SUSPENDED
	syncedAt  time.Time
	createdAt time.Time
}

// NewInquilinoProjection construye la proyección a partir del evento de auth.
func NewInquilinoProjection(userID, email string) *InquilinoProjection {
	now := time.Now()
	return &InquilinoProjection{
		userID:    userID,
		email:     email,
		status:    "ACTIVE",
		syncedAt:  now,
		createdAt: now,
	}
}

// ReconstructInquilinoProjection rehidrata la proyección desde Postgres.
func ReconstructInquilinoProjection(userID, email, status string, syncedAt, createdAt time.Time) *InquilinoProjection {
	return &InquilinoProjection{
		userID:    userID,
		email:     email,
		status:    status,
		syncedAt:  syncedAt,
		createdAt: createdAt,
	}
}

// Suspend marca al inquilino como suspendido — no puede abrir nuevos tickets.
func (i *InquilinoProjection) Suspend() {
	i.status = "SUSPENDED"
	i.syncedAt = time.Now()
}

// Reactivate vuelve a habilitar al inquilino.
func (i *InquilinoProjection) Reactivate() {
	i.status = "ACTIVE"
	i.syncedAt = time.Now()
}

// CanOpenTicket valida si el inquilino está habilitado para abrir tickets.
func (i *InquilinoProjection) CanOpenTicket() bool {
	return i.status == "ACTIVE"
}

// Getters
func (i *InquilinoProjection) UserID() string       { return i.userID }
func (i *InquilinoProjection) Email() string        { return i.email }
func (i *InquilinoProjection) Status() string       { return i.status }
func (i *InquilinoProjection) SyncedAt() time.Time  { return i.syncedAt }
func (i *InquilinoProjection) CreatedAt() time.Time { return i.createdAt }
