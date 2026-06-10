-- +migrate Up
-- Proyección local de inquilinos en inmo_maintenance_db
--
-- Propósito: mantener una copia mínima de los inquilinos que pueden abrir tickets,
-- sincronizada via el evento auth.user.created (cuando role = INQUILINO).
--
-- Datos que NO se almacenan acá (viven en inmo_catalog_db.inquilinos):
--   - CUIL, actividad laboral, ingresos, score crediticio
-- Esos datos se consultan via NATS request/reply solo si maintenance los necesita.

CREATE TABLE IF NOT EXISTS inquilino_projections (
    user_id     VARCHAR(64)  PRIMARY KEY,              -- vínculo lógico con auth_db.users
    email       VARCHAR(255) NOT NULL,                 -- para notificaciones de tickets
    status      VARCHAR(30)  NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, SUSPENDED
    synced_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_inquilino_proj_status ON inquilino_projections(status);