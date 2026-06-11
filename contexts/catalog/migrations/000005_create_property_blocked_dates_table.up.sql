CREATE TABLE IF NOT EXISTS property_blocked_dates (
    id             VARCHAR(64) PRIMARY KEY,
    property_id    VARCHAR(64) NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    start_date     DATE NOT NULL,
    end_date       DATE NOT NULL,
    reason         VARCHAR(20) NOT NULL DEFAULT 'RESERVATION',
    reservation_id VARCHAR(64),
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blocked_dates_property ON property_blocked_dates(property_id);
CREATE INDEX IF NOT EXISTS idx_blocked_dates_range    ON property_blocked_dates(property_id, start_date, end_date);
