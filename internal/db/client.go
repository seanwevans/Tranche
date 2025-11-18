package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	*Queries
	db *sql.DB
}

func Open(ctx context.Context, dsn string) (*DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{Queries: New(db), db: db}, nil
}

func (c *DB) Close() error {
	return c.db.Close()
}
