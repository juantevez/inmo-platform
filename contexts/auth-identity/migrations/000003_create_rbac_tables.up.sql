-- +migrate Up
-- Migración: RBAC granular — permissions y role_permissions
-- Esta migración agrega el soporte para permisos finos tal como describe el documento de diseño.
-- Los permisos ya están embebidos en el JWT (calculados en derivePermissions en main.go),
-- pero esta tabla es la fuente de verdad persistida para auditoría y futura administración dinámica.

-- 1. Tabla de permisos atómicos del sistema
CREATE TABLE IF NOT EXISTS permissions (
    id          VARCHAR(60) PRIMARY KEY,   -- Ej: 'property:create', 'maintenance:read'
    description TEXT,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. Tabla de mapeo rol → permisos
CREATE TABLE IF NOT EXISTS role_permissions (
    role          VARCHAR(50) NOT NULL,    -- Mismo ENUM/VARCHAR que user_roles.role
    permission_id VARCHAR(60) NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role, permission_id)
);

-- 3. Seed: permisos del sistema alineados con la matriz RBAC del documento
INSERT INTO permissions (id, description) VALUES
    ('property:read',        'Ver propiedades del catálogo'),
    ('property:create',      'Publicar nuevas propiedades'),
    ('property:update',      'Editar propiedades existentes'),
    ('property:delete',      'Dar de baja propiedades'),
    ('postulation:create',   'Postularse o consultar sobre una propiedad'),
    ('postulation:read',     'Ver postulaciones recibidas'),
    ('postulation:update',   'Gestionar postulaciones (aprobar/rechazar)'),
    ('contract:read',        'Ver contratos activos'),
    ('contract:create',      'Generar nuevos contratos'),
    ('contract:update',      'Modificar contratos existentes'),
    ('contract:delete',      'Rescindir o eliminar contratos'),
    ('maintenance:create',   'Abrir tickets de reclamo/mantenimiento'),
    ('maintenance:read',     'Ver tickets de mantenimiento'),
    ('maintenance:update',   'Actualizar estado de tickets (presupuesto, cierre)'),
    ('maintenance:delete',   'Eliminar tickets'),
    ('invoice:read',         'Ver facturas y comprobantes'),
    ('invoice:create',       'Generar facturas o liquidaciones'),
    ('invoice:upload',       'Subir comprobantes de pago'),
    ('invoice:update',       'Modificar facturas existentes'),
    ('invoice:delete',       'Eliminar facturas'),
    ('ledger:read',          'Ver caja y liquidaciones'),
    ('ledger:create',        'Registrar movimientos de caja'),
    ('ledger:update',        'Modificar movimientos de caja'),
    ('message:create',       'Enviar mensajes internos'),
    ('message:read',         'Leer mensajes internos'),
    ('tenant:create',        'Crear nuevas inmobiliarias (solo ROOT)'),
    ('tenant:read',          'Ver métricas de inmobiliarias (solo ROOT)'),
    ('tenant:update',        'Modificar datos de inmobiliarias (solo ROOT)'),
    ('tenant:delete',        'Suspender inmobiliarias (solo ROOT)'),
    ('metrics:read',         'Ver métricas globales del SaaS (solo ROOT)')
ON CONFLICT (id) DO NOTHING;

-- 4. Seed: asignación de permisos por rol (espejo de derivePermissions en main.go)

-- INTERESADO: solo puede buscar y postularse
INSERT INTO role_permissions (role, permission_id) VALUES
    ('INTERESADO', 'property:read'),
    ('INTERESADO', 'postulation:create')
ON CONFLICT DO NOTHING;

-- INQUILINO
INSERT INTO role_permissions (role, permission_id) VALUES
    ('INQUILINO', 'property:read'),
    ('INQUILINO', 'contract:read'),
    ('INQUILINO', 'maintenance:create'),
    ('INQUILINO', 'maintenance:read'),
    ('INQUILINO', 'invoice:read'),
    ('INQUILINO', 'invoice:upload'),
    ('INQUILINO', 'postulation:create'),
    ('INQUILINO', 'message:create'),
    ('INQUILINO', 'message:read')
ON CONFLICT DO NOTHING;

-- PROPIETARIO
INSERT INTO role_permissions (role, permission_id) VALUES
    ('PROPIETARIO', 'property:read'),
    ('PROPIETARIO', 'property:create'),
    ('PROPIETARIO', 'property:update'),
    ('PROPIETARIO', 'contract:read'),
    ('PROPIETARIO', 'contract:update'),
    ('PROPIETARIO', 'maintenance:read'),
    ('PROPIETARIO', 'maintenance:update'),
    ('PROPIETARIO', 'invoice:read'),
    ('PROPIETARIO', 'invoice:create'),
    ('PROPIETARIO', 'message:create'),
    ('PROPIETARIO', 'message:read'),
    ('PROPIETARIO', 'ledger:read'),
    ('PROPIETARIO', 'ledger:create')
ON CONFLICT DO NOTHING;

-- AGENTE
INSERT INTO role_permissions (role, permission_id) VALUES
    ('AGENTE', 'property:read'),
    ('AGENTE', 'property:create'),
    ('AGENTE', 'property:update'),
    ('AGENTE', 'property:delete'),
    ('AGENTE', 'postulation:read'),
    ('AGENTE', 'postulation:update'),
    ('AGENTE', 'contract:read'),
    ('AGENTE', 'contract:create'),
    ('AGENTE', 'contract:update'),
    ('AGENTE', 'maintenance:read'),
    ('AGENTE', 'maintenance:update'),
    ('AGENTE', 'invoice:read'),
    ('AGENTE', 'message:create'),
    ('AGENTE', 'message:read')
ON CONFLICT DO NOTHING;

-- PROVEEDOR: solo ve y actualiza tickets asignados
INSERT INTO role_permissions (role, permission_id) VALUES
    ('PROVEEDOR', 'maintenance:read'),
    ('PROVEEDOR', 'maintenance:update')
ON CONFLICT DO NOTHING;

-- ADMIN_INMO: gestión completa dentro de su tenant
INSERT INTO role_permissions (role, permission_id) VALUES
    ('ADMIN_INMO', 'property:read'),
    ('ADMIN_INMO', 'property:create'),
    ('ADMIN_INMO', 'property:update'),
    ('ADMIN_INMO', 'property:delete'),
    ('ADMIN_INMO', 'postulation:read'),
    ('ADMIN_INMO', 'postulation:update'),
    ('ADMIN_INMO', 'contract:read'),
    ('ADMIN_INMO', 'contract:create'),
    ('ADMIN_INMO', 'contract:update'),
    ('ADMIN_INMO', 'contract:delete'),
    ('ADMIN_INMO', 'maintenance:read'),
    ('ADMIN_INMO', 'maintenance:create'),
    ('ADMIN_INMO', 'maintenance:update'),
    ('ADMIN_INMO', 'maintenance:delete'),
    ('ADMIN_INMO', 'invoice:read'),
    ('ADMIN_INMO', 'invoice:create'),
    ('ADMIN_INMO', 'invoice:update'),
    ('ADMIN_INMO', 'invoice:delete'),
    ('ADMIN_INMO', 'ledger:read'),
    ('ADMIN_INMO', 'ledger:create'),
    ('ADMIN_INMO', 'ledger:update'),
    ('ADMIN_INMO', 'message:create'),
    ('ADMIN_INMO', 'message:read')
ON CONFLICT DO NOTHING;

-- ROOT: solo gestión de tenants y métricas globales
INSERT INTO role_permissions (role, permission_id) VALUES
    ('ROOT', 'tenant:create'),
    ('ROOT', 'tenant:read'),
    ('ROOT', 'tenant:update'),
    ('ROOT', 'tenant:delete'),
    ('ROOT', 'metrics:read')
ON CONFLICT DO NOTHING;

-- 5. Índice para consultas rápidas por rol
CREATE INDEX IF NOT EXISTS idx_role_permissions_role ON role_permissions(role);
