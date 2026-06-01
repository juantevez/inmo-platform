ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS operation_type VARCHAR(10) NOT NULL DEFAULT 'SALE';

CREATE INDEX IF NOT EXISTS idx_properties_operation_type ON properties(operation_type);
