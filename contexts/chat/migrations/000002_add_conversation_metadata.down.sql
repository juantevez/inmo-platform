ALTER TABLE conversations
  DROP COLUMN IF EXISTS property_title,
  DROP COLUMN IF EXISTS seeker_name,
  DROP COLUMN IF EXISTS advertiser_name;
