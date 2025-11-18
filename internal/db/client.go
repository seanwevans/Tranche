package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	*Queries
	conn *sql.DB
}

func Open(ctx context.Context, dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{Queries: New(conn), conn: conn}, nil
}

func (dbx *DB) Close() error {
	if dbx == nil || dbx.conn == nil {
		return nil
	}
	return dbx.conn.Close()
}

func (dbx *DB) Conn() *sql.DB {
	return dbx.conn
}
