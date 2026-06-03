-- Migración 1: Tablas principales del bounded context chat

CREATE TABLE IF NOT EXISTS conversations (
    id              VARCHAR(64)  PRIMARY KEY,
    property_id     VARCHAR(64)  NOT NULL,
    seeker_id       VARCHAR(64)  NOT NULL,
    advertiser_id   VARCHAR(64)  NOT NULL,
    lead_id         VARCHAR(64),
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Un hilo único por propiedad + par de participantes
    CONSTRAINT uq_conversation_participants UNIQUE (property_id, seeker_id, advertiser_id)
);

CREATE INDEX IF NOT EXISTS idx_conversations_seeker     ON conversations(seeker_id);
CREATE INDEX IF NOT EXISTS idx_conversations_advertiser ON conversations(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_conversations_updated    ON conversations(updated_at DESC);

-- ------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS chat_messages (
    id              VARCHAR(64)  PRIMARY KEY,
    conversation_id VARCHAR(64)  NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       VARCHAR(64)  NOT NULL,
    msg_type        VARCHAR(30)  NOT NULL,  -- TEXT | VISIT_PROPOSAL | SYSTEM
    body            TEXT         NOT NULL,
    metadata        JSONB        NOT NULL DEFAULT '{}',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_conv ON chat_messages(conversation_id, created_at ASC);

-- ------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS visit_proposals (
    id              VARCHAR(64)  PRIMARY KEY,
    conversation_id VARCHAR(64)  NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    lead_id         VARCHAR(64)  NOT NULL,
    proposed_at     TIMESTAMP WITH TIME ZONE NOT NULL,
    status          VARCHAR(30)  NOT NULL DEFAULT 'PENDING_APPROVAL',  -- PENDING_APPROVAL | ACCEPTED | REJECTED
    resolved_at     TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_visit_proposals_conv   ON visit_proposals(conversation_id);
CREATE INDEX IF NOT EXISTS idx_visit_proposals_lead   ON visit_proposals(lead_id);
CREATE INDEX IF NOT EXISTS idx_visit_proposals_status ON visit_proposals(status);

-- ------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS chat_outbox_events (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    subject      VARCHAR(255) NOT NULL,
    payload      BYTEA        NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_chat_outbox_pending
    ON chat_outbox_events(status, created_at)
    WHERE status = 'PENDING';
