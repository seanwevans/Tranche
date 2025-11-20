package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"tranche/internal/db"
)

type PostgresMetrics struct {
	db                *db.Queries
	emptyAvailability float64
	now               func() time.Time
}

func NewPostgresMetrics(dbx *db.Queries) *PostgresMetrics {
	return &PostgresMetrics{db: dbx, emptyAvailability: 0, now: time.Now}
}

func NewPostgresMetricsWithDefault(dbx *db.Queries, emptyAvailability float64) *PostgresMetrics {
	return &PostgresMetrics{db: dbx, emptyAvailability: emptyAvailability, now: time.Now}
}

func (m *PostgresMetrics) RecordProbe(ctx context.Context, serviceID int64, target string, ok bool, latency time.Duration) error {
	err := m.db.InsertProbeSample(ctx, db.InsertProbeSampleParams{
		ServiceID:  serviceID,
		MetricsKey: target,
		ProbedAt:   m.now(),
		Ok:         ok,
		LatencyMs:  sql.NullInt32{Int32: int32(latency.Milliseconds()), Valid: latency > 0},
	})
	return err
}

func (m *PostgresMetrics) Availability(ctx context.Context, serviceID int64, window time.Duration) (float64, error) {
	cutoff := m.now().Add(-window)
	avail, err := m.db.GetProbeAvailability(ctx, db.GetProbeAvailabilityParams{
		EmptyAvailability: m.emptyAvailability,
		ServiceID:         serviceID,
		Cutoff:            cutoff,
	})
	if err != nil {
		return 0, err
	}

	switch v := avail.(type) {
	case nil:
		return m.emptyAvailability, nil
	case float64:
		return v, nil
	case []byte:
		parsed, perr := strconv.ParseFloat(string(v), 64)
		if perr != nil {
			return 0, fmt.Errorf("parse availability: %w", perr)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected availability type %T", v)
	}
}
