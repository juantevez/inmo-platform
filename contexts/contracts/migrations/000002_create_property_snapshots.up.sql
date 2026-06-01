-- Mirror local de datos de Catálogo que Contratos necesita para validar reservas sin
-- depender de una llamada HTTP sincrónica a Catálogo en el momento de la reserva.
CREATE TABLE IF NOT EXISTS property_snapshots (
    property_id       VARCHAR(64) PRIMARY KEY,
    owner_id          VARCHAR(64) NOT NULL,
    operation_type    VARCHAR(20) NOT NULL DEFAULT 'TEMP',
    night_price       NUMERIC(12, 2) NOT NULL DEFAULT 0,
    cleaning_fee      NUMERIC(12, 2) NOT NULL DEFAULT 0,
    security_deposit  NUMERIC(12, 2) NOT NULL DEFAULT 0,
    min_nights        INTEGER NOT NULL DEFAULT 1,
    max_nights        INTEGER NOT NULL DEFAULT 90,
    check_in_time     VARCHAR(10) NOT NULL DEFAULT '14:00',
    check_out_time    VARCHAR(10) NOT NULL DEFAULT '10:00',
    pricing_rules     JSONB,
    updated_at        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
