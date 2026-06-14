-- Seed de usuarios de prueba — todos con password: Inmo1234!
-- Hash bcrypt (cost=10): $2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC

-- ============================================================
-- 1. USUARIOS
-- ============================================================
INSERT INTO users (id, email, status, phone) VALUES
    ('user-admin-001',        'sofia.gomez@tevezprop.com',        'ACTIVE', '+5491133441100'),
    ('user-agente-001',       'laura.martinez@tevezprop.com',     'ACTIVE', '+5491144552211'),
    ('user-propietario-001',  'roberto.fernandez@gmail.com',      'ACTIVE', '+5491155663322'),
    ('user-propietario-002',  'ana.beltran@gmail.com',            'ACTIVE', '+5491166774433'),
    ('user-inquilino-001',    'diego.suarez@gmail.com',           'ACTIVE', '+5491177885544'),
    ('user-interesado-001',   'valentina.rossi@gmail.com',        'ACTIVE', '+5491188996655'),
    ('user-proveedor-001',    'hector.rodriguez@plomeria.com',    'ACTIVE', '+5491199007766')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 2. ROLES
-- ============================================================
INSERT INTO user_roles (user_id, role) VALUES
    ('user-admin-001',       'ADMIN_INMO'),
    ('user-agente-001',      'AGENTE'),
    ('user-propietario-001', 'PROPIETARIO'),
    ('user-propietario-002', 'PROPIETARIO'),
    ('user-inquilino-001',   'INQUILINO'),
    ('user-interesado-001',  'INTERESADO'),
    ('user-proveedor-001',   'PROVEEDOR')
ON CONFLICT (user_id, role) DO NOTHING;

-- ============================================================
-- 3. IDENTITY PROVIDERS (EMAIL, password: Inmo1234!)
-- ============================================================
INSERT INTO identity_providers (id, user_id, provider_name, provider_user_id, password_hash) VALUES
    ('idp-admin-001',        'user-admin-001',        'EMAIL', 'sofia.gomez@tevezprop.com',     '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-agente-001',       'user-agente-001',       'EMAIL', 'laura.martinez@tevezprop.com',  '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-propietario-001',  'user-propietario-001',  'EMAIL', 'roberto.fernandez@gmail.com',   '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-propietario-002',  'user-propietario-002',  'EMAIL', 'ana.beltran@gmail.com',         '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-inquilino-001',    'user-inquilino-001',    'EMAIL', 'diego.suarez@gmail.com',        '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-interesado-001',   'user-interesado-001',   'EMAIL', 'valentina.rossi@gmail.com',     '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC'),
    ('idp-proveedor-001',    'user-proveedor-001',    'EMAIL', 'hector.rodriguez@plomeria.com', '$2b$10$8dWUqw3DuqNUaiuWpeJ0UeC/RJeKOTzzww.5CtyTpQd2gwR89hPoC')
ON CONFLICT (id) DO NOTHING;
