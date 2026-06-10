-- +migrate Up
-- Proyección local de propiedades en inmo_maintenance_db
--
-- Propósito: guardar los datos MÍNIMOS que maintenance necesita para operar
-- de forma autónoma sin consultar catalog en cada operación de ticket.
--
-- Esta tabla se alimenta del evento catalog.property.published via NATS.
-- Para datos en tiempo real (dirección, nombre del propietario), maintenance
-- consulta catalog via NATS request/reply únicamente cuando el técnico lo necesita.
--
-- Relación con tickets: tickets.property_id → property_projections.property_id (lógica, sin FK física)

CREATE TABLE IF NOT EXISTS property_projections (
    property_id     VARCHAR(64)  PRIMARY KEY,
    owner_id        VARCHAR(64)  NOT NULL,        -- para saber a quién notificar al aprobar presupuesto
    operation_type  VARCHAR(20)  NOT NULL,         -- SALE | RENT | TEMP
    state           VARCHAR(30)  NOT NULL,         -- espejo del estado en catalog
    tenant_id       VARCHAR(64),                   -- inmobiliaria responsable (puede ser null si dueño directo)
    synced_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Índice para consultas por propietario (ej: listar tickets del propietario)
CREATE INDEX IF NOT EXISTS idx_property_proj_owner ON property_projections(owner_id);

-- Índice para filtrar por estado (ej: solo propiedades AVAILABLE o UNDER_REPAIR)
CREATE INDEX IF NOT EXISTS idx_property_proj_state ON property_projections(state);
