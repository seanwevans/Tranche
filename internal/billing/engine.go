package billing

import (
	"context"
	"time"

	"tranche/internal/db"
)

type Logger interface {
	Printf(string, ...any)
}

type Engine struct {
	db  *db.DB
	log Logger
}

func NewEngine(dbx *db.DB, log Logger) *Engine {
	return &Engine{db: dbx, log: log}
}

func (e *Engine) RunOnce(ctx context.Context, now time.Time) error {
	// TODO: join CDN usage with storm_events and write invoices
	e.log.Printf("billing run at %s (stub)", now.Format(time.RFC3339))
	return nil
}
