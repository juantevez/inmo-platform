# Contracts Context

Manages two types of agreements over real estate properties:

1. **Traditional contracts** — long-term rentals with a full state machine and periodic price adjustments (ICL / IPC / FIX).
2. **Temporary reservations** — short-term bookings with owner approval workflow.

This context does **not** call the Catalog service directly. It consumes property data via `PropertySnapshot` objects received from NATS events.

## Domain

### Aggregates

**Contract** (long-term rental)

```
DRAFT → ACTIVE → RENEWED → TERMINATED
              ↘              ↗
               TERMINATED (directly)
```

**Reservation** (temporary stay)

```
PENDING_APPROVAL → CONFIRMED → ACTIVE → COMPLETED
                             ↘
                              CANCELLED
```

### Value Objects

| Object | Responsibility |
|---|---|
| `RentAmount` | Amount + currency, must be > 0 |
| `Timeline` | Start/end dates; end must be after start |
| `AdjustmentIndex` | `ICL`, `IPC`, or `FIX` |
| `AdjustmentPeriod` | Months between adjustments |
| `PropertySnapshot` | Cached copy of pricing/config from Catalog at booking time |

### Domain Events

- `ContractActivated` — triggers property state change in Catalog.
- `ReservationCreated` — notifies interested contexts of a new booking.
- `ReservationConfirmed` / `ReservationCancelled` — status transitions.

## Architecture

```
cmd/api/           → entry point
internal/
  domain/          → aggregates, value objects, state machine
  ports/           → repository interfaces
  application/     → use cases
  adapters/
    postgres/      → PostgreSQL + Outbox Worker
    nats/          → NATS JetStream subscribers (property snapshots)
    httpapi/       → HTTP handlers
migrations/        → SQL migrations (golang-migrate)
```

### Outbox Pattern

Same dual-write pattern as Catalog: events are persisted in `contracts_outbox_events` alongside the domain object in the same transaction. The `OutboxWorker` scans every 5 seconds and publishes to NATS.

### Property Snapshots

When Catalog publishes `catalog.property.published` or `catalog.property.updated`, the `PropertySubscriber` caches the data in `property_snapshots`. All reservation calculations (price, discounts, availability) use this local copy, avoiding runtime coupling to the Catalog service.

## Use Cases

| Use Case | Description |
|---|---|
| `CreateContract` | Creates a contract in `DRAFT` state |
| `ActivateContract` | Transitions to `ACTIVE`, publishes `ContractActivated` |
| `CreateReservation` | Validates dates against snapshot, calculates total cost |
| `ConfirmReservation` | Owner approves a pending booking |
| `CancelReservation` | Cancels a pending or confirmed booking |
| `GetReservation` | Retrieves reservation details |
| `GetOwnerReservations` | Lists all reservations for an owner |

## HTTP API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/contracts` | Required | Create contract |
| `POST` | `/api/v1/contracts/activate` | Required | Activate contract |
| `POST` | `/api/v1/reservations` | Required | Create reservation |
| `POST` | `/api/v1/reservations/{id}/confirm` | Required | Confirm booking |
| `POST` | `/api/v1/reservations/{id}/cancel` | Required | Cancel booking |
| `GET` | `/api/v1/reservations/{id}` | Required | Reservation details |
| `GET` | `/api/v1/reservations/owner/{ownerID}` | Required | Owner's reservations |

## NATS Events

**Publishes** (subject prefix `contracts.*`):
- `contracts.contract.activated`
- `contracts.reservation.created`
- `contracts.reservation.confirmed`
- `contracts.reservation.cancelled`

**Subscribes**:
- `catalog.property.published` — caches property snapshot.
- `catalog.property.updated` — updates existing snapshot.

## Database

PostgreSQL — `inmo_catalog_db` (separate migrations table from Catalog). Main tables:

| Table | Description |
|---|---|
| `contracts` | Long-term rental agreements with state |
| `reservations` | Temporary bookings with full pricing snapshot |
| `property_snapshots` | Cached property data received from Catalog |
| `contracts_outbox_events` | Event log for the Outbox pattern |

### Migrations

```bash
migrate -path migrations/ -database "$DATABASE_URL" up
```

## Running Locally

```bash
# Dependencies (PostgreSQL + NATS)
docker compose -f deploy/docker/docker-compose.yml up -d postgres nats

# Service
go run ./contexts/contracts/cmd/api
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | PostgreSQL connection string |
| `NATS_URL` | `nats://localhost:4222` | NATS address |

## Tests

```bash
make test
make test-cover
```
