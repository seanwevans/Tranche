package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps a sql.DB connection and exposes the generated query methods.
type DB struct {
	*Queries
	conn *sql.DB
}

// Open connects to Postgres using the pgx stdlib driver and prepares the
// generated query helpers.
func Open(ctx context.Context, dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{
		Queries: New(conn),
		conn:    conn,
	}, nil
}

// Close releases the underlying sql.DB connection.
func (db *DB) Close() error {
	if db == nil || db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

// Conn exposes the underlying *sql.DB for callers that need raw access.
func (db *DB) Conn() *sql.DB {
	if db == nil {
		return nil
	}
	return db.conn
}
