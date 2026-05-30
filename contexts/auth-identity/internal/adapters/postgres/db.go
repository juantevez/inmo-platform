package postgres

import (
	"database/sql"
	"fmt"
	"time"
)

type DB struct {
	Pool *sql.DB
}

func NewDB(connString string) (*DB, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, fmt.Errorf("error al abrir la base de datos: %w", err)
	}

	// Configuraciones de producción para el pool de conexiones
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("no se pudo conectar a Postgres (Ping fallido): %w", err)
	}

	return &DB{Pool: db}, nil
}
