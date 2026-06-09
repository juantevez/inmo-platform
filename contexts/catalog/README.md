# Catalog Context

Manages the lifecycle of real estate properties: publication, state transitions, pricing, media, and owner profiles. Supports three operation types: permanent sale (`SALE`), permanent rent (`RENT`), and temporary short-term rentals (`TEMP`).

## Domain

### Aggregates

- **Property** — the core listing. Possible states: `AVAILABLE → RESERVED → CLOSED / UNDER_REPAIR`.
- **Profile** — business profile of the property owner. States: `PENDING_VERIFICATION → ACTIVE / SUSPENDED`.

### Value Objects

| Object | Responsibility |
|---|---|
| `Price` | Amount + currency (`ARS`/`USD`), must be > 0 |
| `Location` | Lat/lon + address with geographic bounds validation |
| `TempConfig` | Config exclusive to `TEMP` properties: night price, fees, min/max nights, check-in/out times, amenities, and dynamic pricing rules |

### Domain Events

- `PropertyPublished` — fired after publication; carries a `PropertySnapshot` for downstream contexts.
- `PropertyUpdated` — any pricing or amenity change.
- `PropertyStateChanged` — any state transition.

## Architecture

Follows hexagonal architecture (ports & adapters).

```
cmd/api/           → entry point
internal/
  domain/          → aggregates, value objects, domain events
  ports/           → repository and provider interfaces
  application/     → use cases
  adapters/
    postgres/      → PostgreSQL + Outbox Worker
    nats/          → NATS JetStream subscribers
    s3/            → S3 pre-signed URLs for media upload
    inmemory/      → in-memory implementations (testing)
    httpapi/       → HTTP handlers
migrations/        → SQL migrations (golang-migrate)
```

### Outbox Pattern

Publication uses a **transactional outbox**: property and event are persisted in the same transaction. A background worker (`OutboxWorker`) scans the `outbox_events` table every 20 seconds with `FOR UPDATE SKIP LOCKED` and publishes to NATS JetStream, guaranteeing at-least-once delivery.

## Use Cases

| Use Case | Description |
|---|---|
| `PublishProperty` | Creates a property with optional `TempConfig` |
| `ChangePropertyState` | Valid state transitions (reserve, close, repair) |
| `ListProperties` | Paginated list with filters (state, type, pet policy, price) |
| `QuoteProperty` | Calculates total cost for a temporary stay (nights, fees, discounts, availability) |
| `CreateProfile` | Creates owner business profile |
| `AddPropertyMedia` | Attaches images/videos/social links |
| `GenerateUploadURL` | Generates S3 pre-signed URL for direct client upload |

## HTTP API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/properties` | Required | Publish property |
| `GET` | `/api/v1/properties` | No | List with filters |
| `POST` | `/api/v1/properties/{id}/reserve` | Required | Reserve |
| `POST` | `/api/v1/properties/{id}/quote` | Required | Price quote |
| `POST` | `/api/v1/properties/{id}/media/upload-url` | Required | Pre-signed URL |
| `POST` | `/api/v1/properties/{id}/media` | Required | Register media |
| `GET` | `/api/v1/properties/{id}/media` | No | List media |
| `POST` | `/api/v1/catalog/profiles` | Required | Create profile |
| `GET` | `/api/v1/catalog/profiles/me` | Required | Get my profile |
| `GET` | `/healthz/live` | No | Liveness |
| `GET` | `/healthz/ready` | No | Readiness |

## NATS Events

**Publishes** (subject prefix `catalog.property.*`):
- `catalog.property.published`
- `catalog.property.updated`
- `catalog.property.state_changed`

**Subscribes**:
- `contracts.property.*` — updates property snapshots when a contract is activated.

## Database

PostgreSQL — `inmo_catalog_db`. Main tables:

| Table | Description |
|---|---|
| `properties` | Core listings with operation type, pet policy, state |
| `outbox_events` | Event log for the Outbox pattern |
| `profiles` | Owner business profiles |
| `property_media` | Media with sort order |
| `blocked_dates` | Availability calendar for temporary rentals |

### Migrations

```bash
migrate -path migrations/ -database "$DATABASE_URL" up
```

## Running Locally

```bash
# Dependencies (PostgreSQL + NATS)
docker compose -f deploy/docker/docker-compose.yml up -d postgres nats

# Service
go run ./contexts/catalog/cmd/api
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | PostgreSQL connection string |
| `NATS_URL` | `nats://localhost:4222` | NATS address |
| `AWS_BUCKET_NAME` | — | S3 bucket (optional) |
| `AWS_REGION` | — | AWS region (optional) |

## Tests

```bash
# From repo root
make test
make test-cover
```
