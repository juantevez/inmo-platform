# API Gateway

Single entry point for the entire platform. Handles:

- **JWT authentication** — validates `Authorization: Bearer <token>` and injects `X-User-Id` / `X-User-Role` headers upstream.
- **CORS** — centralized policy; removes duplicate headers before forwarding.
- **Routing** — reverse-proxies each request to the correct microservice.
- **WebSocket tunneling** — full-duplex TCP tunnel (connection hijacking, not HTTP upgrade) for the Chat service.

No business logic lives here. It is a thin infrastructure layer.

## Architecture

```
cmd/gateway/           → entry point
internal/
  config/              → env-based configuration
  middleware/
    auth.go            → JWT validation
    cors.go            → CORS headers
  proxy/
    router.go          → route table + reverse proxies
```

## Route Table

### Public (no authentication required)

| Method | Path | Upstream |
|---|---|---|
| `POST` | `/api/v1/auth/login` | auth-identity |
| `POST` | `/api/v1/auth/register` | auth-identity |
| `GET` | `/api/v1/auth/verify` | auth-identity |
| `POST` | `/api/v1/auth/sso/*` | auth-identity |
| `GET` | `/api/v1/properties` | catalog |
| `GET` | `/api/v1/properties/{id}` | catalog |
| `GET` | `/api/v1/properties/{id}/media` | catalog |

### Private (Bearer token required)

| Path prefix | Upstream |
|---|---|
| `/api/v1/properties` (write) | catalog |
| `/api/v1/catalog/profiles` | catalog |
| `/api/v1/contracts/*` | contracts |
| `/api/v1/reservations/*` | contracts |
| `/api/v1/leads/*` | crm |
| `/api/v1/tickets/*` | maintenance |
| `/api/v1/finances/*` | finances |
| `/api/v1/chats/*` | chat |
| `/ws/chats/*` | chat (WebSocket, token via query param) |

## Configuration

All values are read from environment variables.

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_PORT` | `:8000` | Listening port |
| `JWT_SECRET` | `dev_secret_local` | HS256 signing key |
| `CATALOG_SERVICE_URL` | `http://127.0.0.1:8081` | Catalog upstream |
| `CONTRACTS_SERVICE_URL` | `http://127.0.0.1:8085` | Contracts upstream |
| `AUTH_SERVICE_URL` | `http://127.0.0.1:8080` | Auth upstream |
| `MAINTENANCE_SERVICE_URL` | `http://127.0.0.1:8085` | Maintenance upstream |
| `FINANCES_SERVICE_URL` | `http://127.0.0.1:8082` | Finances upstream |
| `CRM_SERVICE_URL` | `http://127.0.0.1:8084` | CRM upstream |
| `CHAT_SERVICE_URL` | `http://127.0.0.1:8086` | Chat upstream |

## Running Locally

```bash
go run ./contexts/api-gateway/cmd/gateway
```

For the full stack:

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

## Tests

```bash
make test
make test-cover
```
