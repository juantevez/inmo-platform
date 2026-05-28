package ddd

// AggregateRoot define el comportamiento para cualquier raíz de agregado que maneje eventos.
type AggregateRoot struct {
	events []DomainEvent
}

// RecordEvent registra un evento de dominio en el agregado.
func (a *AggregateRoot) RecordEvent(event DomainEvent) {
	if a.events == nil {
		a.events = make([]DomainEvent, 0)
	}
	a.events = append(a.events, event)
}

// PullEvents retorna todos los eventos acumulados y limpia la lista interna.
// Patrón fundamental para la publicación post-persistencia (Transactional Outbox implícito).
func (a *AggregateRoot) PullEvents() []DomainEvent {
	events := a.events
	a.events = make([]DomainEvent, 0)
	return events
}
