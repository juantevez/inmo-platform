#!/bin/bash
set -e

echo "🐳 Creando bases de datos adicionales..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE inmo_maintenance_db;
    GRANT ALL PRIVILEGES ON DATABASE auth_db TO inmo_user;
    GRANT ALL PRIVILEGES ON DATABASE inmo_maintenance_db TO inmo_user;
EOSQL