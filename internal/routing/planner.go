package routing

import (
	"context"

	"tranche/internal/db"
)

type Weights struct {
	Primary int
	Backup  int
}

type Planner struct {
	db *db.Queries
}

func NewPlanner(dbx *db.Queries) *Planner {
	return &Planner{db: dbx}
}

func (p *Planner) DesiredRouting(ctx context.Context, serviceID int64) (Weights, error) {
	storms, err := p.db.GetActiveStormsForService(ctx, serviceID)
	if err != nil {
		return Weights{}, err
	}
	if len(storms) > 0 {
		return Weights{Primary: 0, Backup: 100}, nil
	}
	return Weights{Primary: 100, Backup: 0}, nil
}
