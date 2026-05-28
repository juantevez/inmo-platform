package ddd

import "time"

// DomainEvent define la interfaz que cualquier evento de dominio debe implementar.
type DomainEvent interface {
	EventID() string
	AggregateID() string
	EventName() string
	OccurredAt() time.Time
}

// BaseDomainEvent provee una estructura común para no repetir campos en cada evento.
type BaseDomainEvent struct {
	ID        string    `json:"event_id"`
	TargetID  string    `json:"aggregate_id"`
	Name      string    `json:"event_name"`
	Timestamp time.Time `json:"occurred_at"`
}

func NewBaseDomainEvent(id, aggregateID, name string) BaseDomainEvent {
	return BaseDomainEvent{
		ID:        id,
		TargetID:  aggregateID,
		Name:      name,
		Timestamp: time.Now().UTC(),
	}
}

func (e BaseDomainEvent) EventID() string       { return e.ID }
func (e BaseDomainEvent) AggregateID() string   { return e.TargetID }
func (e BaseDomainEvent) EventName() string     { return e.Name }
func (e BaseDomainEvent) OccurredAt() time.Time { return e.Timestamp }
