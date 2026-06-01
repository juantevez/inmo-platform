CREATE TABLE IF NOT EXISTS reservations (
    id                      VARCHAR(64) PRIMARY KEY,
    property_id             VARCHAR(64) NOT NULL,
    tenant_id               VARCHAR(64) NOT NULL,
    owner_id                VARCHAR(64) NOT NULL,
    check_in_date           DATE NOT NULL,
    check_out_date          DATE NOT NULL,
    nights                  INTEGER NOT NULL,
    night_price_snapshot    NUMERIC(12, 2) NOT NULL,
    discount_pct            NUMERIC(5, 2) NOT NULL DEFAULT 0,
    cleaning_fee            NUMERIC(12, 2) NOT NULL DEFAULT 0,
    security_deposit        NUMERIC(12, 2) NOT NULL DEFAULT 0,
    total_amount            NUMERIC(12, 2) NOT NULL,
    status                  VARCHAR(30) NOT NULL DEFAULT 'PENDING_APPROVAL',
    guest_message           TEXT,
    confirmed_at            TIMESTAMP WITH TIME ZONE,
    cancelled_at            TIMESTAMP WITH TIME ZONE,
    created_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_reservations_property ON reservations(property_id);
CREATE INDEX idx_reservations_tenant   ON reservations(tenant_id);
CREATE INDEX idx_reservations_status   ON reservations(status);
CREATE INDEX idx_reservations_dates    ON reservations(property_id, check_in_date, check_out_date);
