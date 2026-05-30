-- +migrate Up

-- 1. Tabla Núcleo del Usuario (Corregida sin duplicar credenciales)
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(50) NOT NULL, -- 'PENDING_VERIFICATION', 'ACTIVE', 'SUSPENDED'
    phone VARCHAR(50) DEFAULT '',
    phone_verified_at TIMESTAMP WITH TIME ZONE NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. Tabla de Credenciales / Métodos de Ingreso (Se mantiene igual, impecable)
CREATE TABLE IF NOT EXISTS identity_providers (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    provider_name VARCHAR(50) NOT NULL, -- 'EMAIL', 'GOOGLE', 'META'
    provider_user_id VARCHAR(255) NOT NULL, 
    password_hash VARCHAR(255) NULL, -- NULL si entró por SSO puro (Manejado correctamente acá)
    CONSTRAINT fk_identity_providers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_provider_key UNIQUE (provider_name, provider_user_id)
);

-- 3. Tabla de Tokens Temporales
CREATE TABLE IF NOT EXISTS verification_tokens (
    token VARCHAR(255) PRIMARY KEY,
    token_type VARCHAR(50) NOT NULL, 
    user_id VARCHAR(64) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT fk_verification_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- 4. Índices de Optimización
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_identity_providers_user_id ON identity_providers(user_id);
CREATE INDEX IF NOT EXISTS idx_verification_tokens_lookup ON verification_tokens(token, token_type);