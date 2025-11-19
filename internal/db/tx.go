package db

import (
	"context"
	"database/sql"
	"fmt"
)

type txStarter interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// BeginTx starts a transaction and returns a Queries instance bound to it.
func (q *Queries) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Queries, *sql.Tx, error) {
	starter, ok := q.db.(txStarter)
	if !ok {
		return nil, nil, fmt.Errorf("db does not support transactions")
	}
	tx, err := starter.BeginTx(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	return q.WithTx(tx), tx, nil
}
