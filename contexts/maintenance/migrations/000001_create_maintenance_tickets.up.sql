-- Creación de la tabla principal para el Agregado Ticket de Mantenimiento
CREATE TABLE IF NOT EXISTS tickets (
    id VARCHAR(36) PRIMARY KEY,
    property_id VARCHAR(36) NOT NULL,
    tenant_id VARCHAR(36) NOT NULL,
    provider_id VARCHAR(36), -- Se llena al pasar a VALIDATED
    description TEXT NOT NULL,
    status VARCHAR(20) NOT NULL, -- OPEN, VALIDATED, QUOTED, APPROVED, IN_PROGRESS, CLOSED
    urgency VARCHAR(20) NOT NULL, -- EMERGENCY, URGENT, SCHEDULED
    
    -- Bloque de Presupuesto (Quote) - Opcionales hasta estado QUOTED
    quote_amount DECIMAL(12, 2),
    quote_details TEXT,
    quote_at TIMESTAMP WITH TIME ZONE,
    
    -- Bloque de Evidencia de Cierre (Evidence) - Opcionales hasta estado CLOSED
    evidence_description TEXT,
    evidence_url TEXT, -- Link al bucket S3 de AWS
    
    -- Trazabilidad de tiempos
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    closed_at TIMESTAMP WITH TIME ZONE
);

-- Índices estratégicos para optimizar búsquedas frecuentes
CREATE INDEX IF NOT EXISTS idx_tickets_property ON tickets(property_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_tenant ON tickets(tenant_id);
