# InmoPlatform - Event-Driven Monorepo en Go

Plataforma inmobiliaria distribuida de alta disponibilidad diseñada bajo los principios de **Domain-Driven Design (DDD)**, **Arquitectura Hexagonal (Ports & Adapters)** y **Event-Driven Architecture (EDA)**. 

El sistema implementa consistencia eventual y resiliencia extrema mediante el patrón **Transactional Outbox**, garantizando la entrega de mensajes (*At-Least-Once delivery*) ante fallos de red o caídas del Message Broker.

---

## Arquitectura General y Flujo de Datos

El proyecto está estructurado como un monorepo multi-módulo en Go que divide las fronteras del negocio en **Bounded Contexts** totalmente desacoplados a nivel de base de datos y lógica, comunicados de forma asincrónica.

### Componentes de Infraestructura:
* **API Gateway / HTTP Router:** Punto de entrada síncrono para los clientes del sistema.
* **PostgreSQL:** Persistencia transaccional ACID (bases de datos independientes por contexto).
* **NATS JetStream (v2):** Mensajería persistente con soporte para *Durable Consumers*.
* **Outbox Worker (Relay):** Proceso en segundo plano nativo en Go (`goroutine` + `time.Ticker`) encargado del vaciado atómico de la bandeja de salida.

---

##  Estructura del Proyecto (Monorepo)

```text
inmo-platform/
├── go.work                  # Go Workspace global
├── shared/                  # Shared Kernel (Módulo reutilizable)
│   ├── pkg/
│   │   ├── ddd/             # Abstracciones de Dominio (AggregateRoot, DomainEvent)
│   │   ├── apperr/          # Manejo semántico de errores
│   │   ├── pg/              # Pool de conexiones optimizado para Postgres
│   │   └── eventbus/        # Conector core de NATS JetStream v2
│   └── go.mod
└── contexts/
    ├── auth-identity/       # Bounded Context de Autenticación e Identidad
    │   ├── cmd/api/         # Composition Root (main.go)
    │   ├── migrations/      # Scripts SQL (Tablas users, identity_providers, verification_tokens)
    │   └── internal/
    │       ├── domain/      # Agregado User, IdentityProvider, VerificationToken
    │       ├── ports/       # Contratos: UserRepository, TokenRepository, IdentityService
    │       ├── application/ # Casos de Uso: Register, Login, VerifyEmail, SSO Google/Meta
    │       └── adapters/    # HTTP Handlers, Postgres Repo, Redis Token Store, OAuth adapters
    ├── contracts/           # Bounded Context de Contratos Inmobiliarios
    │   ├── cmd/api/         # Composition Root (main.go)
    │   ├── migrations/      # Scripts SQL (Tabla contracts)
    │   └── internal/
    │       ├── domain/      # Agregado Contract, máquina de estados (DRAFT→ACTIVE→RENEWED/TERMINATED)
    │       ├── ports/       # Contratos: ContractRepository
    │       ├── application/ # Casos de Uso: CreateContract, ActivateContract
    │       └── adapters/    # HTTP Handlers, Postgres Repo, Outbox Worker
    ├── catalog/             # Bounded Context de Catálogo Inmobiliario
    │   ├── cmd/api/         # Composition Root (main.go)
    │   ├── migrations/      # Scripts SQL (Tablas properties y outbox_events)
    │   └── internal/
    │       ├── domain/      # Corazón de negocio (Invariantes de Property, Price, Location)
    │       ├── ports/       # Contratos/Interfaces (PropertyRepository)
    │       ├── application/ # Casos de Uso transaccionales (PublishProperty)
    │       └── adapters/    # HTTP Handlers, Postgres Repo y el Outbox Worker
    ├── crm/                 # Bounded Context de CRM & Leads
    │   ├── cmd/api/         # Composition Root (main.go)
    │   ├── migrations/      # Scripts SQL (Tabla leads)
    │   └── internal/
    │       ├── domain/      # Agregado Lead y Máquina de Estados (NEW -> CONTACTED)
    │       ├── ports/       # Interfaces de salida
    │       ├── application/ # Casos de uso asincrónicos (CreateAutoLead)
    │       └── adapters/    # Suscriptor Durable de NATS JetStream y Postgres Repo
    ├── finances/            # Bounded Context de Finanzas & Liquidaciones
    │   ├── cmd/api/         # Composition Root (main.go, puerto :8082)
    │   ├── migrations/      # Scripts SQL (Tablas settlements y settlement_concepts)
    │   └── internal/
    │       ├── domain/      # Agregado Settlement, Entidad Concept, Máquina de estados (OPEN→CLOSED→PAID)
    │       ├── ports/       # Contratos: SettlementRepository, ContractService, EventDispatcher
    │       ├── application/ # Casos de Uso: CreateSettlement, AddConcept, CloseSettlement
    │       └── adapters/    # HTTP Handlers, Postgres Repo, Stubs de servicios externos
    └── maintenance/         # Bounded Context de Mantenimiento & Incidencias
        ├── cmd/api/         # Composition Root (main.go, puerto :8083)
        ├── migrations/      # Scripts SQL (Tabla tickets)
        └── internal/
            ├── domain/      # Agregado Ticket, Value Objects Quote y Evidence, máquina de estados
            ├── ports/       # Contratos: TicketRepository, CatalogService, EventDispatcher
            ├── application/ # Casos de Uso: CreateTicket, AssignProvider, SubmitQuote, ApproveTicket, CloseTicket
            └── adapters/    # HTTP Handlers, Postgres Repo, Stubs de servicios externos
```

---

## Bounded Context: auth-identity

Gestiona el ciclo de vida completo de la identidad del usuario bajo arquitectura hexagonal. Expone autenticación tradicional (email/password) y SSO con proveedores externos.

### Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| `POST` | `/auth/register` | Registro con email y contraseña |
| `POST` | `/auth/login` | Login tradicional, retorna access + refresh token |
| `GET` | `/auth/verify` | Verificación de email por token |
| `POST` | `/auth/sso/google` | Login/Registro vía Google OAuth2 |
| `POST` | `/auth/sso/meta` | Login/Registro vía Meta (Facebook/Instagram) |

### SSO con Google

El frontend obtiene el `authorization_code` desde el SDK de Google y lo envía al backend. El backend lo intercambia por el perfil del usuario contra la API de Google.

```bash
POST http://localhost:8080/auth/sso/google
Content-Type: application/json

{
  "code": "<authorization_code_de_google>"
}
```

**Respuesta exitosa:**
```json
{
  "access_token": "jwt.mock.access_token.for_user_<id>",
  "refresh_token": "<token_64_bytes>"
}
```

### SSO con Meta (Facebook / Instagram)

El frontend obtiene el `access_token` de corta duración directamente desde el SDK de Facebook Login y lo envía al backend. El backend lo valida contra la Graph API de Meta (`/me?fields=id,name,email,picture`).

```bash
POST http://localhost:8080/auth/sso/meta
Content-Type: application/json

{
  "access_token": "<access_token_de_meta>"
}
```

**Respuesta exitosa:**
```json
{
  "access_token": "jwt.mock.access_token.for_user_<id>",
  "refresh_token": "<token_64_bytes>"
}
```

**Escenarios manejados automáticamente:**

- **Usuario nuevo:** se registra con status `ACTIVE` (Meta ya verificó su identidad).
- **Login recurrente:** valida que el `provider_user_id` de Meta coincida con el registrado.
- **Account linking:** si el email ya existe por otro proveedor (email/Google), vincula Meta a la cuenta existente (requiere que el usuario tenga status `ACTIVE`).
- **Sin email en Meta:** retorna `422` pidiendo al frontend solicitar el email al usuario en un paso adicional.

### Infraestructura de auth-identity

- **PostgreSQL** (`auth_db`): tablas `users`, `identity_providers`, `verification_tokens`.
- **Redis**: almacén de refresh tokens (TTL 7 días) y rate limiting de login (5 intentos / 15 min por IP+email).
- **NATS JetStream**: publicación de eventos `auth.user.created` y `auth.user.logged_in` para auditoría y notificaciones asíncronas.

---

## Bounded Context: contracts

Gestiona el ciclo de vida de los contratos inmobiliarios con una máquina de estados estricta y publicación de eventos vía Transactional Outbox.

### Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| `POST` | `/api/v1/contracts` | Crear un contrato nuevo (estado inicial `DRAFT`) |
| `POST` | `/api/v1/contracts/activate` | Activar un contrato (transición `DRAFT → ACTIVE`) |

### Máquina de Estados

```
DRAFT ──► ACTIVE ──► RENEWED
                └──► TERMINATED
```

- **DRAFT:** estado inicial al crear el contrato (pendiente de firma).
- **ACTIVE:** contrato vigente (firmado por ambas partes).
- **RENEWED:** contrato activo que fue renovado al vencer.
- **TERMINATED:** estado final, no admite más transiciones.

---

## Bounded Context: finances

Gestiona las liquidaciones mensuales de los contratos inmobiliarios. Permite crear una liquidación por período, cargar conceptos detallados (alquiler, impuestos, servicios, expensas) y cerrarla para su emisión.

Corre de forma independiente en el **puerto `:8082`**.

### Endpoints

| Método | Ruta | Descripción |
|--------|------|-------------|
| `POST` | `/api/v1/settlements/create` | Crear una liquidación nueva (estado inicial `OPEN`) |
| `POST` | `/api/v1/settlements/concepts/add` | Agregar un concepto a una liquidación abierta |
| `POST` | `/api/v1/settlements/close` | Cerrar la liquidación (`OPEN → CLOSED`) |

### Máquina de Estados

```
OPEN ──► CLOSED ──► PAID
```

- **OPEN:** estado inicial; acepta conceptos.
- **CLOSED:** liquidación emitida; no admite más modificaciones.
- **PAID:** estado final tras el cobro/pago efectivo.

### Tipos de Concepto soportados

| Tipo | Descripción |
|------|-------------|
| `RENT` | Alquiler mensual |
| `TAX` | Impuestos (ABL, Inmobiliario, etc.) |
| `EXPENSES` | Expensas de la propiedad |
| `ADJUSTMENT` | Ajuste por índice o corrección |
| `UTILITY_ELECTRICITY` | Servicio eléctrico |
| `UTILITY_WATER` | Servicio de agua |
| `UTILITY_GAS` | Servicio de gas |
| `UTILITY_INTERNET` | Internet |
| `UTILITY_CABLE_TV` | Cable / TV paga |

### Infraestructura de finances

- **PostgreSQL** (`inmo_catalog_db`): tablas `settlements` y `settlement_concepts`.
  - Invariante a nivel BD: un contrato solo puede tener una liquidación por período (`UNIQUE contract_id + period`).
- **ContractService / EventDispatcher:** implementados como stubs; preparados para integración real con los contextos `contracts` y el bus de eventos.

### Migración

```bash
docker exec -i inmo-postgres psql -U inmo_user -d inmo_catalog_db < contexts/finances/migrations/000001_create_finances_settlements.up.sql
```

### Ejecución en Desarrollo

```bash
go run ./contexts/finances/cmd/api
```

### Prueba de Integración

```bash
# 1. Crear una liquidación para el período 2026-05
curl -X POST http://localhost:8082/api/v1/settlements/create \
  -H "Content-Type: application/json" \
  -d '{"settlement_id":"liq-001","contract_id":"contract-abc","period":"2026-05"}'

# 2. Agregar el concepto de alquiler
curl -X POST http://localhost:8082/api/v1/settlements/concepts/add \
  -H "Content-Type: application/json" \
  -d '{"settlement_id":"liq-001","concept_id":"con-001","description":"Alquiler Mayo 2026","concept_type":"RENT","amount":250000}'

# 3. Agregar expensas
curl -X POST http://localhost:8082/api/v1/settlements/concepts/add \
  -H "Content-Type: application/json" \
  -d '{"settlement_id":"liq-001","concept_id":"con-002","description":"Expensas Mayo","concept_type":"EXPENSES","amount":45000}'

# 4. Cerrar la liquidación
curl -X POST http://localhost:8082/api/v1/settlements/close \
  -H "Content-Type: application/json" \
  -d '{"settlement_id":"liq-001"}'
```

### Comprobación en Base de Datos

```bash
# Verificar la liquidación creada
docker exec -it inmo-postgres psql -U inmo_user -d inmo_catalog_db -c "SELECT id, contract_id, period, status FROM settlements;"

# Verificar los conceptos cargados
docker exec -it inmo-postgres psql -U inmo_user -d inmo_catalog_db -c "SELECT id, description, concept_type, amount FROM settlement_concepts;"
```

---

## Flujo del Patrón Transactional Outbox
Para evitar la pérdida de eventos de dominio si el broker de mensajería se cae, el sistema no publica directamente en NATS desde el caso de uso, sino que delega la atomicidad a la base de datos:

1. El cliente envía un POST /api/v1/properties.

2. El caso de uso inicia una Transacción SQL.

3. Se inserta la propiedad en la tabla properties Y el evento en la tabla outbox_events con estado PENDING.

4. Se ejecuta el COMMIT. (Si algo falla, se hace ROLLBACK y nada se persiste).

5. En paralelo, el Outbox Worker escanea la tabla cada 20 segundos usando FOR UPDATE SKIP LOCKED para evitar colisiones concurrentes.

6. El Worker publica el evento en el Subject catalog.property.published de NATS.

7. Al recibir el OK del broker, actualiza la fila en la BD a PROCESSED.

8. El servicio de CRM, mediante un consumidor duradero (crm-auto-captacion), procesa el mensaje y genera un Lead automático.


### Requisitos Previos y Configuración
Asegurate de tener instalado:

- Go 1.25

### Docker y Docker Compose

1. Levantar la Infraestructura (Postgres & NATS)
En la raíz del proyecto, encendé los contenedores necesarios:

```
docker compose up -d
```


### 2. Ejecutar las Migraciones SQL
Crea las tablas necesarias en la base de datos ejecutando los scripts en orden:

```
# Catálogo (Estructura base y Outbox)
docker exec -i inmo-postgres psql -U inmo_user -d inmo_catalog_db < contexts/catalog/migrations/000001_create_properties_table.up.sql
docker exec -i inmo-postgres psql -U inmo_user -d inmo_catalog_db < contexts/catalog/migrations/000002_create_outbox_table.up.sql

# CRM (Estructura de Leads)
docker exec -i inmo-postgres psql -U inmo_user -d inmo_catalog_db < contexts/crm/migrations/000001_create_leads_table.up.sql
```

### Ejecución en Desarrollo
Al ser servicios independientes comunicados por eventos, debés levantar ambos módulos en terminales separadas:

#### Terminal 1: Iniciar Módulo CRM (Suscriptor)

```
go run ./contexts/crm/cmd/api
```
### Pruebas de Integración (Prueba de Fuego)
Envía una petición HTTP para publicar una propiedad en el contexto de Catálogo:
```
curl -X POST http://localhost:8080/api/v1/properties \
  -H "Content-Type: application/json" \
  -d '{
    "id": "prop-readme-100",
    "owner_id": "owner-juan-20",
    "title": "Ph 3 Ambientes Sin Expensas",
    "description": "Hermoso Ph en planta baja, patio con parrilla.",
    "price": 75000,
    "currency": "USD",
    "latitude": -34.6460,
    "longitude": -58.5620,
    "address": "Necochea 400, Ramos Mejia"
  }'
```
### Comprobación de Consistencia Asincrónica:
Verificá en la base de datos cómo impactaron ambas capas de persistencia y el historial del Outbox:
```
# Verificar registro en Catálogo
docker exec -it inmo-postgres psql -U inmo_user -d inmo_catalog_db -c "SELECT id, title, price FROM properties WHERE id='prop-readme-100';"

# Verificar estado de procesamiento en la bandeja de salida (Outbox)
docker exec -it inmo-postgres psql -U inmo_user -d inmo_catalog_db -c "SELECT id, event_name, status FROM outbox_events;"

# Verificar creación automática del Lead en el contexto de CRM
docker exec -it inmo-postgres psql -U inmo_user -d inmo_catalog_db -c "SELECT id, property_id, client_name, state FROM leads;"
```
