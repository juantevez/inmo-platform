-- Campos para alquiler temporario (aplican cuando operation_type = 'TEMP')
ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS amenities         JSONB,
    ADD COLUMN IF NOT EXISTS check_in_time     TIME    NOT NULL DEFAULT '14:00',
    ADD COLUMN IF NOT EXISTS check_out_time    TIME    NOT NULL DEFAULT '10:00',
    ADD COLUMN IF NOT EXISTS min_nights        INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS max_nights        INTEGER NOT NULL DEFAULT 90,
    ADD COLUMN IF NOT EXISTS night_price       NUMERIC(12, 2),
    ADD COLUMN IF NOT EXISTS cleaning_fee      NUMERIC(12, 2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS security_deposit  NUMERIC(12, 2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pricing_rules     JSONB;

-- Tabla de fechas bloqueadas para disponibilidad del calendario
CREATE TABLE IF NOT EXISTS property_blocked_dates (
    id              VARCHAR(64) PRIMARY KEY,
    property_id     VARCHAR(64) NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    start_date      DATE NOT NULL,
    end_date        DATE NOT NULL,
    reason          VARCHAR(20) NOT NULL DEFAULT 'RESERVATION',
    reservation_id  VARCHAR(64),
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blocked_dates_property  ON property_blocked_dates(property_id);
CREATE INDEX IF NOT EXISTS idx_blocked_dates_range     ON property_blocked_dates(property_id, start_date, end_date);
