-- Perfiles de los usuarios seed creados en auth-identity (000004_seed_test_users.up.sql)
-- Todos con status ACTIVE para que el onboarding no vuelva a pedirles datos

INSERT INTO profiles (user_id, first_name, last_name, dni_cuit, phone, profile_type, company_name, license_number, status) VALUES

    -- ADMIN_INMO: trabaja en la inmobiliaria
    ('user-admin-001',
     'Sofia', 'Gómez', '27-33441100-5', '+5491133441100',
     'COMMERCIAL', 'Tevez Propiedades', NULL, 'ACTIVE'),

    -- AGENTE: tiene matrícula CUCICBA
    ('user-agente-001',
     'Laura', 'Martínez', '27-28552211-3', '+5491144552211',
     'COMMERCIAL', 'Tevez Propiedades', 'CUCICBA Matricula N° 6534', 'ACTIVE'),

    -- PROPIETARIO 1: dueño de PROP-001 a PROP-010
    ('user-propietario-001',
     'Roberto', 'Fernández', '20-25663322-7', '+5491155663322',
     'INDIVIDUAL', NULL, NULL, 'ACTIVE'),

    -- PROPIETARIA 2: dueña de PROP-011 a PROP-020
    ('user-propietario-002',
     'Ana', 'Beltrán', '27-29774433-1', '+5491166774433',
     'INDIVIDUAL', NULL, NULL, 'ACTIVE'),

    -- INQUILINO
    ('user-inquilino-001',
     'Diego', 'Suárez', '20-32885544-9', '+5491177885544',
     'INDIVIDUAL', NULL, NULL, 'ACTIVE'),

    -- INTERESADO
    ('user-interesado-001',
     'Valentina', 'Rossi', '27-37996655-2', '+5491188996655',
     'INDIVIDUAL', NULL, NULL, 'ACTIVE'),

    -- PROVEEDOR: tiene empresa de plomería
    ('user-proveedor-001',
     'Héctor', 'Rodríguez', '30-71234567-8', '+5491199007766',
     'COMMERCIAL', 'Rodríguez Plomería', NULL, 'ACTIVE')

ON CONFLICT (user_id) DO NOTHING;
