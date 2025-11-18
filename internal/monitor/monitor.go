package monitor

import (
	"context"
	"net/http"
	"time"

	"tranche/internal/db"
)

type Logger interface {
	Printf(string, ...any)
}

type MetricsRecorder interface {
	RecordProbe(serviceID int64, ok bool, latency time.Duration)
}

type MetricsView struct {
	rec *InMemoryMetrics
}

func NewMetricsView(rec *InMemoryMetrics) *MetricsView {
	return &MetricsView{rec: rec}
}

func (mv *MetricsView) Availability(serviceID int64, window time.Duration) (float64, error) {
	return mv.rec.Availability(serviceID, window), nil
}

type Scheduler struct {
	db  *db.DB
	m   MetricsRecorder
	log Logger
}

func NewScheduler(dbx *db.DB, mr MetricsRecorder, log Logger) *Scheduler {
	return &Scheduler{db: dbx, m: mr, log: log}
}

func (s *Scheduler) Run(ctx context.Context) {
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		services, err := s.db.GetActiveServices(ctx)
		if err != nil {
			s.log.Printf("GetActiveServices: %v", err)
			return
		}
		for _, svc := range services {
			go s.probeLoop(ctx, client, svc.ID)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (s *Scheduler) probeLoop(ctx context.Context, client *http.Client, serviceID int64) {
	for {
		start := time.Now()
		ok := false
		// TODO: make probe URL configurable per service/domain
		resp, err := client.Get("https://example.com/healthz")
		if err == nil && resp.StatusCode < 500 {
			ok = true
		}
		lat := time.Since(start)
		s.m.RecordProbe(serviceID, ok, lat)

		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}
