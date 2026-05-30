-- Up Migration: Estructura para el Bounded Context de Finanzas

-- 1. Tabla Raíz del Agregado (Cabecera)
CREATE TABLE IF NOT EXISTS settlements (
    id VARCHAR(36) PRIMARY KEY,
    contract_id VARCHAR(36) NOT NULL,
    period VARCHAR(7) NOT NULL, -- Formato "YYYY-MM" (e.g., "2026-05")
    status VARCHAR(20) NOT NULL, -- "OPEN", "CLOSED", "PAID"
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    closed_at TIMESTAMP WITH TIME ZONE,
    
    -- Invariante a nivel de persistencia: Un contrato solo tiene una liquidación por mes
    CONSTRAINT uk_contract_period UNIQUE (contract_id, period)
);

-- 2. Tabla Entidad Interna (Detalle de Conceptos)
CREATE TABLE IF NOT EXISTS settlement_concepts (
    id VARCHAR(36) PRIMARY KEY,
    settlement_id VARCHAR(36) NOT NULL,
    description VARCHAR(255) NOT NULL,
    concept_type VARCHAR(50) NOT NULL, -- "RENT", "TAX", "UTILITY_GAS", etc.
    amount DECIMAL(12, 2) NOT NULL,
    
    -- Si se borra la liquidación raíz, se limpian sus conceptos automáticamente
    CONSTRAINT fk_concepts_settlement FOREIGN KEY (settlement_id) 
        REFERENCES settlements(id) ON DELETE CASCADE
);

-- 3. Índice para acelerar la búsqueda de liquidaciones por contrato
CREATE INDEX IF NOT EXISTS idx_settlements_contract ON settlements(contract_id);
