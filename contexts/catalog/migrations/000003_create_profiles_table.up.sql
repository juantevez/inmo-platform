CREATE TABLE IF NOT EXISTS profiles (
    user_id       VARCHAR(64) NOT NULL,
    first_name    VARCHAR(100) NOT NULL,
    last_name     VARCHAR(100) NOT NULL,
    dni_cuit      VARCHAR(20) NOT NULL UNIQUE,
    phone         VARCHAR(50),
    profile_type  VARCHAR(20) NOT NULL DEFAULT 'INDIVIDUAL',
    company_name  VARCHAR(150),
    license_number VARCHAR(50),
    status        VARCHAR(30) NOT NULL DEFAULT 'PENDING_VERIFICATION',
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id)
);

CREATE INDEX idx_profiles_status  ON profiles(status);
CREATE INDEX idx_profiles_license ON profiles(license_number) WHERE license_number IS NOT NULL;
