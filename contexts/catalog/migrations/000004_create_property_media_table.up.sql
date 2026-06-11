CREATE TABLE IF NOT EXISTS property_media (
    id           VARCHAR(64) PRIMARY KEY,
    property_id  VARCHAR(64) NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    url          TEXT,
    type         VARCHAR(20) NOT NULL,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    social_links JSONB,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_property_media_property_id ON property_media(property_id);
