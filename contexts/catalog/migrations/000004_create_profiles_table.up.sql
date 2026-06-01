CREATE TABLE IF NOT EXISTS profiles (
    -- El user_id viene de auth_db, acá funciona como PK para asegurar la relación 1 a 1 por usuario
    user_id VARCHAR(64) NOT NULL,
    
    -- Datos de contacto/identificación básicos de negocio
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    dni_cuit VARCHAR(20) NOT NULL UNIQUE,
    phone VARCHAR(50),
    
    -- Flexibilidad estructural basada en tu investigación:
    -- 'INDIVIDUAL' (Dueño directo) o 'COMMERCIAL' (Inmobiliaria, Agente, Martillero)
    profile_type VARCHAR(20) NOT NULL DEFAULT 'INDIVIDUAL',
    
    -- Campos exclusivos para perfiles comerciales/profesionales (pueden ser NULL si es dueño directo)
    company_name VARCHAR(150), -- Nombre de fantasía de la inmobiliaria
    license_number VARCHAR(50), -- Matrícula del corredor o martillero público
    
    -- Auditoría y control de estado
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING_VERIFICATION', -- PENDING_VERIFICATION, ACTIVE, SUSPENDED
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    PRIMARY KEY (user_id)
);

-- Índices estratégicos para búsquedas rápidas en el panel de administración
CREATE INDEX idx_profiles_status ON profiles(status);
CREATE INDEX idx_profiles_license ON profiles(license_number) WHERE license_number IS NOT NULL;
