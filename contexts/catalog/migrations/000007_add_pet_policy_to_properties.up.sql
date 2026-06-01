ALTER TABLE properties
    ADD COLUMN IF NOT EXISTS pet_policy VARCHAR(20) NOT NULL DEFAULT 'NOT_ALLOWED';

CREATE INDEX IF NOT EXISTS idx_properties_pet_policy ON properties(pet_policy);
