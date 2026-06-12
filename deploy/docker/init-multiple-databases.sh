#!/bin/bash
set -e

echo "🐳 Creando bases de datos adicionales..."

# 1️⃣ Crear las bases primero (desde la base por defecto)
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE inmo_maintenance_db;
    GRANT ALL PRIVILEGES ON DATABASE auth_db TO inmo_user;
    GRANT ALL PRIVILEGES ON DATABASE inmo_maintenance_db TO inmo_user;
EOSQL

echo "🗺️ Habilitando PostGIS en inmo_catalog_db..."

# 2️⃣ PostGIS en inmo_catalog_db (propiedades, CRM, chat, contratos, finances)
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "inmo_catalog_db" <<-EOSQL
    CREATE EXTENSION IF NOT EXISTS postgis;
    CREATE EXTENSION IF NOT EXISTS postgis_topology;
EOSQL

echo "✅ Bases de datos e extensiones listas."