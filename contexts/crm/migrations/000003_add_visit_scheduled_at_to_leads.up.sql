ALTER TABLE leads ADD COLUMN IF NOT EXISTS visit_scheduled_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_leads_visit_scheduled_at ON leads(visit_scheduled_at) WHERE visit_scheduled_at IS NOT NULL;
