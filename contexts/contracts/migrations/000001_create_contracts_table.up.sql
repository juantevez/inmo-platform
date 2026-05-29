-- 1. Tabla Principal de Contratos
CREATE TABLE IF NOT EXISTS contracts (
    id VARCHAR(64) PRIMARY KEY,
    property_id VARCHAR(64) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    owner_id VARCHAR(64) NOT NULL,
    rent_amount NUMERIC(12, 2) NOT NULL,
    currency VARCHAR(10) NOT NULL,
    start_date TIMESTAMP WITH TIME ZONE NOT NULL,
    end_date TIMESTAMP WITH TIME ZONE NOT NULL,
    adjustment_index VARCHAR(20) NOT NULL,
    adjustment_period_months INT NOT NULL,
    state VARCHAR(30) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Índices para optimizar las búsquedas de contratos por entidad y estado
CREATE INDEX IF NOT EXISTS idx_contracts_property ON contracts(property_id);
CREATE INDEX IF NOT EXISTS idx_contracts_tenant ON contracts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_contracts_state ON contracts(state);

-- 2. Tabla de Outbox para el Contexto de Contratos
CREATE TABLE IF NOT EXISTS contracts_outbox_events (
    id VARCHAR(64) PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id VARCHAR(64) NOT NULL,
    event_name VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE
);

-- Índice parcial para que el Outbox Worker escanee a la velocidad de la luz
CREATE INDEX IF NOT EXISTS idx_contracts_outbox_status ON contracts_outbox_events(status) WHERE status = 'PENDING';
