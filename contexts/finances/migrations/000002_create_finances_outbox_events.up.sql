-- Up Migration: Tabla de Outbox para el módulo de Finanzas
CREATE TABLE IF NOT EXISTS finances_outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_name VARCHAR(255) NOT NULL,
    payload BYTEA NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_finances_outbox_pending 
ON finances_outbox_events(status, created_at) 
WHERE status = 'PENDING';
