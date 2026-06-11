CREATE TABLE IF NOT EXISTS properties (
    id               VARCHAR(64) PRIMARY KEY,
    owner_id         VARCHAR(64) NOT NULL,
    title            VARCHAR(255) NOT NULL,
    description      TEXT,
    price            NUMERIC(12, 2) NOT NULL,
    currency         VARCHAR(10) NOT NULL,
    latitude         NUMERIC(9, 6) NOT NULL,
    longitude        NUMERIC(9, 6) NOT NULL,
    address          VARCHAR(255) NOT NULL,
    state            VARCHAR(50) NOT NULL,
    operation_type   VARCHAR(10) NOT NULL DEFAULT 'SALE',
    pet_policy       VARCHAR(20) NOT NULL DEFAULT 'NOT_ALLOWED',
    amenities        JSONB,
    check_in_time    TIME NOT NULL DEFAULT '14:00',
    check_out_time   TIME NOT NULL DEFAULT '10:00',
    min_nights       INTEGER NOT NULL DEFAULT 1,
    max_nights       INTEGER NOT NULL DEFAULT 90,
    night_price      NUMERIC(12, 2),
    cleaning_fee     NUMERIC(12, 2) NOT NULL DEFAULT 0,
    security_deposit NUMERIC(12, 2) NOT NULL DEFAULT 0,
    pricing_rules    JSONB,
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_properties_state          ON properties(state);
CREATE INDEX IF NOT EXISTS idx_properties_operation_type ON properties(operation_type);
CREATE INDEX IF NOT EXISTS idx_properties_pet_policy     ON properties(pet_policy);
