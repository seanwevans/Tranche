package monitor

import (
	"context"
	"net/http"
	"sync"
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
	db    *db.DB
	m     MetricsRecorder
	log   Logger
	mu    sync.Mutex
	loops map[int64]context.CancelFunc
}

func NewScheduler(dbx *db.DB, mr MetricsRecorder, log Logger) *Scheduler {
	return &Scheduler{db: dbx, m: mr, log: log, loops: make(map[int64]context.CancelFunc)}
}

func (s *Scheduler) Run(ctx context.Context) {
	client := &http.Client{Timeout: 5 * time.Second}
	defer s.cancelAllLoops()

	for {
		services, err := s.db.GetActiveServices(ctx)
		if err != nil {
			s.log.Printf("GetActiveServices: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		active := make(map[int64]struct{}, len(services))
		for _, svc := range services {
			active[svc.ID] = struct{}{}
			s.ensureProbeLoop(ctx, client, svc.ID)
		}
		s.stopMissingLoops(active)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (s *Scheduler) ensureProbeLoop(ctx context.Context, client *http.Client, serviceID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.loops[serviceID]; ok {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	s.loops[serviceID] = cancel
	go s.probeLoop(loopCtx, client, serviceID)
}

func (s *Scheduler) stopMissingLoops(active map[int64]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.loops {
		if _, ok := active[id]; ok {
			continue
		}
		cancel()
		delete(s.loops, id)
	}
}

func (s *Scheduler) cancelAllLoops() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.loops {
		cancel()
		delete(s.loops, id)
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
