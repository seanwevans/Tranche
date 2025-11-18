package storm

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"tranche/internal/db"
)

type stormStore interface {
	GetActiveServices(ctx context.Context) ([]db.GetActiveServicesRow, error)
	GetStormPoliciesForService(ctx context.Context, serviceID int64) ([]db.GetStormPoliciesForServiceRow, error)
	GetActiveStormForPolicy(ctx context.Context, arg db.GetActiveStormForPolicyParams) (db.StormEvent, error)
	GetLastStormEvent(ctx context.Context, arg db.GetLastStormEventParams) (db.StormEvent, error)
	InsertStormEvent(ctx context.Context, arg db.InsertStormEventParams) (db.StormEvent, error)
	MarkStormEventResolved(ctx context.Context, arg db.MarkStormEventResolvedParams) (db.StormEvent, error)
}

type MetricsView interface {
	Availability(serviceID int64, window time.Duration) (float64, error)
}

type Logger interface {
	Printf(string, ...any)
}

type Engine struct {
	db  stormStore
	mv  MetricsView
	log Logger
	now func() time.Time
}

func NewEngine(dbx stormStore, mv MetricsView, log Logger) *Engine {
	return &Engine{db: dbx, mv: mv, log: log, now: time.Now}
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
	window := time.Duration(p.WindowSeconds) * time.Second
	avail, err := e.mv.Availability(serviceID, window)
	if err != nil {
		return err
	}

	activeStorm, err := e.db.GetActiveStormForPolicy(ctx, db.GetActiveStormForPolicyParams{ServiceID: serviceID, Kind: p.Kind})
	hasActive := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	now := e.now()
	cooldown := time.Duration(p.CooldownSeconds) * time.Second

	if avail < p.ThresholdAvail {
		if hasActive {
			return nil
		}

		lastStorm, err := e.db.GetLastStormEvent(ctx, db.GetLastStormEventParams{ServiceID: serviceID, Kind: p.Kind})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && cooldown > 0 {
			if lastStorm.EndedAt.Valid {
				if now.Sub(lastStorm.EndedAt.Time) < cooldown {
					return nil
				}
			} else if now.Sub(lastStorm.StartedAt) < cooldown {
				return nil
			}
		}

		_, err = e.db.InsertStormEvent(ctx, db.InsertStormEventParams{ServiceID: serviceID, Kind: p.Kind})
		return err
	}

	if hasActive {
		_, err = e.db.MarkStormEventResolved(ctx, db.MarkStormEventResolvedParams{ID: activeStorm.ID, EndedAt: sql.NullTime{Time: now, Valid: true}})
		return err
	}

	return nil
}
