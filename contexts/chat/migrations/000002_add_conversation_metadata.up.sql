-- Migración 2: Denormalizar título de propiedad y nombres de participantes
-- para poder mostrar conversaciones identificables sin llamadas cross-service.

ALTER TABLE conversations
  ADD COLUMN IF NOT EXISTS property_title   TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS seeker_name      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS advertiser_name  TEXT NOT NULL DEFAULT '';
