CREATE TABLE IF NOT EXISTS maintenance_outbox_events (
    id BIGSERIAL PRIMARY KEY,
    event_name VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP
);
