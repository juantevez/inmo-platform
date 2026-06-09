# Shared Kernel

Reusable packages shared across all bounded contexts. Contains no business logic — only infrastructure primitives and DDD building blocks.

## Packages

### `pkg/ddd`

Foundation for Domain-Driven Design patterns.

**`AggregateRoot`**

Base struct embedded in all aggregate roots (`Property`, `Contract`, `Reservation`, etc.).

```go
type AggregateRoot struct { ... }

func (a *AggregateRoot) RecordEvent(event DomainEvent)
func (a *AggregateRoot) PullEvents() []DomainEvent
```

`PullEvents()` returns all accumulated events and clears the internal list. The application layer calls this after a use case completes to hand events to the `EventPublisher`.

**`DomainEvent` interface**

```go
type DomainEvent interface {
    EventID()      string
    AggregateID()  string
    EventName()    string
    OccurredAt()   time.Time
}
```

`BaseDomainEvent` is a concrete implementation used as an embedded base in all domain events.

---

### `pkg/apperr`

Semantic application errors that map directly to HTTP status codes.

```go
// Error types
NOT_FOUND             → 404
BAD_REQUEST           → 400
PRECONDITION_FAILED   → 412
FORBIDDEN             → 403
INTERNAL_SERVER_ERROR → 500
```

**Constructors**

```go
apperr.NewNotFound("property not found", err)
apperr.NewBadRequest("invalid currency", err)
apperr.NewPreconditionFailed("property already reserved", err)
apperr.NewForbidden("access denied", err)
apperr.NewInternal("db failure", err)
```

**HTTP translation**

```go
status := apperr.HTTPStatusCode(err) // returns the int for http.Error()
```

All constructors wrap the underlying error so `errors.Unwrap()` chains work correctly.

---

### `pkg/eventbus`

NATS JetStream integration for publishing domain events.

**`JetStreamConnection`**

Wraps `nats.Conn` + `jetstream.JetStream`. Auto-reconnects with exponential backoff (max 5 retries, 2-second wait).

```go
conn, err := eventbus.NewJetStreamConnection(natsURL)
conn.EnsureStream(ctx, streamName, subjects)  // idempotent stream creation
conn.Close()
```

**`EventPublisher`**

Serializes domain events to JSON and publishes to NATS using the event name as subject (e.g., `catalog.property.published`).

```go
publisher := eventbus.NewEventPublisher(conn)
err := publisher.Publish(ctx, events...)
```

---

### `pkg/pg`

PostgreSQL connection pool factory.

```go
pool, err := pg.NewPool(pg.Config{
    URL:          os.Getenv("DATABASE_URL"),
    MaxOpenConns: 25,
    MaxIdleConns: 5,
    MaxIdleTime:  5 * time.Minute,
})
```

---

### `pkg/health`

HTTP health check handlers used by every service.

- `GET /healthz/live` — liveness: always 200 if the process is running.
- `GET /healthz/ready` — readiness: checks PostgreSQL connectivity, NATS status, and background worker health.

```go
checker := health.NewChecker(db, natsConn, workers...)
mux.Handle("/healthz/live", checker.LiveHandler())
mux.Handle("/healthz/ready", checker.ReadyHandler())
```

## Usage in Go Workspace

All contexts import these packages directly via the `go.work` workspace — no versioning or publishing needed.

```go
import (
    "github.com/your-org/inmo-platform/shared/pkg/apperr"
    "github.com/your-org/inmo-platform/shared/pkg/ddd"
    "github.com/your-org/inmo-platform/shared/pkg/eventbus"
)
```
