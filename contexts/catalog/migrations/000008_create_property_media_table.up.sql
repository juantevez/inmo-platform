CREATE TABLE property_media (
    id          VARCHAR(64) PRIMARY KEY,
    property_id VARCHAR(64) NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    url         TEXT,
    type        VARCHAR(20) NOT NULL,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    social_links JSONB,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_property_media_property_id ON property_media(property_id);
