package pg

import "time"

// OutboxEvent representa la estructura exacta de un registro en la tabla de outbox.
type OutboxEvent struct {
	ID        string    `db:"id"`
	Subject   string    `db:"subject"`
	Payload   []byte    `db:"payload"`
	Status    string    `db:"status"` // PENDING, PROCESSED, FAILED
	CreatedAt time.Time `db:"created_at"`
}
