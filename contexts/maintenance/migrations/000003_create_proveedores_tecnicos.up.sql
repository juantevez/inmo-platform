-- +migrate Up
-- Migración: Padrón de Proveedores Técnicos
-- Los proveedores son actores externos (plomeros, electricistas, etc.) que resuelven
-- los tickets de mantenimiento. Su user_id es el vínculo lógico con auth_db.

-- 1. ENUM de rubros técnicos habilitados en el sistema
DO $$ BEGIN
    CREATE TYPE rubro_tecnico AS ENUM (
        'PLOMERO',
        'ELECTRICISTA',
        'ALBANIL',
        'GASISTA',
        'CERRAJERO',
        'PINTOR',
        'CARPINTERO',
        'HERRERO',
        'TECHISTA',
        'JARDINERO',
        'AIRE_ACONDICIONADO',
        'ASCENSORES',
        'LIMPIEZA',
        'OTROS'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL; -- idempotente si se corre dos veces
END $$;

-- 2. Tabla principal de proveedores técnicos
CREATE TABLE IF NOT EXISTS proveedores_tecnicos (
    id              VARCHAR(36)     PRIMARY KEY,
    user_id         VARCHAR(64)     UNIQUE NOT NULL,   -- vínculo lógico con auth_db.users
    razon_social    VARCHAR(100)    NOT NULL,           -- nombre del profesional o empresa
    cuit_cuil       VARCHAR(11)     UNIQUE NOT NULL,
    rubro           rubro_tecnico   NOT NULL,
    cbu_pago        VARCHAR(22),                        -- para liquidarle los trabajos aprobados
    alias_pago      VARCHAR(50),
    disponible_urgencias BOOLEAN    DEFAULT FALSE,      -- acepta tickets EMERGENCY fuera de horario
    status          VARCHAR(20)     NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, SUSPENDED, INACTIVE
    registered_by   VARCHAR(64),                        -- user_id de quien lo registró (ADMIN o él mismo)
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 3. Índices para búsquedas frecuentes
-- Buscar proveedores por rubro es la operación más común al asignar un ticket
CREATE INDEX IF NOT EXISTS idx_proveedores_rubro    ON proveedores_tecnicos(rubro);
CREATE INDEX IF NOT EXISTS idx_proveedores_status   ON proveedores_tecnicos(status);
CREATE INDEX IF NOT EXISTS idx_proveedores_urgencias ON proveedores_tecnicos(disponible_urgencias)
    WHERE disponible_urgencias = TRUE; -- índice parcial: solo los que aceptan urgencias
    