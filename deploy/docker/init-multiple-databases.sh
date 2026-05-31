#!/bin/bash
set -e

echo "🐳 Creando bases de datos adicionales..."
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    GRANT ALL PRIVILEGES ON DATABASE auth_db TO $POSTGRES_USER;

    CREATE DATABASE inmo_catalog_db;
    GRANT ALL PRIVILEGES ON DATABASE inmo_catalog_db TO $POSTGRES_USER;

    CREATE DATABASE inmo_maintenance_db;
    GRANT ALL PRIVILEGES ON DATABASE inmo_maintenance_db TO $POSTGRES_USER;
EOSQL
