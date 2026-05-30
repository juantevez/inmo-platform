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
    ├── catalog/             # Bounded Context de Catálogo Inmobiliario
    │   ├── cmd/api/         # Composition Root (main.go)
    │   ├── migrations/      # Scripts SQL (Tablas properties y outbox_events)
    │   └── internal/
    │       ├── domain/      # Corazón de negocio (Invariantes de Property, Price, Location)
    │       ├── ports/       # Contratos/Interfaces (PropertyRepository)
    │       ├── application/ # Casos de Uso transaccionales (PublishProperty)
    │       └── adapters/    # HTTP Handlers, Postgres Repo y el Outbox Worker
    └── crm/                 # Bounded Context de CRM & Leads
        ├── cmd/api/         # Composition Root (main.go)
        ├── migrations/      # Scripts SQL (Tabla leads)
        └── internal/
            ├── domain/      # Agregado Lead y Máquina de Estados (NEW -> CONTACTED)
            ├── ports/       # Interfaces de salida
            ├── application/ # Casos de uso asincrónicos (CreateAutoLead)
            └── adapters/    # Suscriptor Durable de NATS JetStream y Postgres Repo
```

Flujo del Patrón Transactional Outbox
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
