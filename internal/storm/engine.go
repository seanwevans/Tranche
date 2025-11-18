package storm

import (
	"context"
	"time"

	"tranche/internal/db"
)

type MetricsView interface {
	Availability(serviceID int64, window time.Duration) (float64, error)
}

type Logger interface {
	Printf(string, ...any)
}

type Engine struct {
	db  *db.DB
	mv  MetricsView
	log Logger
}

func NewEngine(dbx *db.DB, mv MetricsView, log Logger) *Engine {
	return &Engine{db: dbx, mv: mv, log: log}
}

func (e *Engine) Tick(ctx context.Context) error {
	services, err := e.db.GetActiveServices(ctx)
	if err != nil {
		return err
	}
	for _, s := range services {
		policies, err := e.db.GetStormPoliciesForService(ctx, s.ID)
		if err != nil {
			return err
		}
		for _, p := range policies {
			if err := e.evaluatePolicy(ctx, s.ID, p); err != nil {
				e.log.Printf("evaluatePolicy(service=%d): %v", s.ID, err)
			}
		}
	}
	return nil
}

func (e *Engine) evaluatePolicy(ctx context.Context, serviceID int64, p db.GetStormPoliciesForServiceRow) error {
	avail, err := e.mv.Availability(serviceID, time.Duration(p.WindowSeconds)*time.Second)
	if err != nil {
		return err
	}
	if avail < p.ThresholdAvail {
		_, err := e.db.InsertStormEvent(ctx, db.InsertStormEventParams{
			ServiceID: serviceID,
			Kind:      p.Kind,
		})
		return err
	}
	return nil
}
