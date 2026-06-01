DROP TABLE IF EXISTS property_blocked_dates;

ALTER TABLE properties
    DROP COLUMN IF EXISTS pricing_rules,
    DROP COLUMN IF EXISTS security_deposit,
    DROP COLUMN IF EXISTS cleaning_fee,
    DROP COLUMN IF EXISTS night_price,
    DROP COLUMN IF EXISTS max_nights,
    DROP COLUMN IF EXISTS min_nights,
    DROP COLUMN IF EXISTS check_out_time,
    DROP COLUMN IF EXISTS check_in_time,
    DROP COLUMN IF EXISTS amenities;
