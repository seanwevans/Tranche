package usageingestor

import (
	"context"
	"fmt"
	"log"
	"time"

	"tranche/internal/cdn"
	"tranche/internal/db"
)

type Engine struct {
	queries  *db.Queries
	provider cdn.Provider
	logger   *log.Logger

	window   time.Duration
	lookback time.Duration
}

func NewEngine(queries *db.Queries, provider cdn.Provider, logger *log.Logger, window, lookback time.Duration) *Engine {
	return &Engine{
		queries:  queries,
		provider: provider,
		logger:   logger,
		window:   window,
		lookback: lookback,
	}
}

func (e *Engine) RunOnce(ctx context.Context, now time.Time) error {
	if e.window <= 0 {
		return fmt.Errorf("window must be positive")
	}

	alignedNow := now.Truncate(e.window)
	windowStart := alignedNow.Add(-e.lookback)

	services, err := e.queries.GetActiveServices(ctx)
	if err != nil {
		return fmt.Errorf("fetch services: %w", err)
	}
	if len(services) == 0 {
		return nil
	}

	domainMap, hostToService, err := e.loadDomains(ctx, services)
	if err != nil {
		return err
	}
	if len(hostToService) == 0 {
		e.logger.Printf("no service domains configured; skipping usage ingestion")
		return nil
	}

	hosts := make([]string, 0, len(hostToService))
	for h := range hostToService {
		hosts = append(hosts, h)
	}

	usages, err := e.provider.Usage(ctx, windowStart, alignedNow, e.window, hosts)
	if err != nil {
		return fmt.Errorf("fetch usage: %w", err)
	}

	aggregates := make(map[usageKey]db.UpsertUsageSnapshotParams)
	for _, u := range usages {
		svcID, ok := hostToService[u.Host]
		if !ok {
			e.logger.Printf("usage for unknown host %s", u.Host)
			continue
		}
		if u.WindowStart.Truncate(e.window) != u.WindowStart || !u.WindowEnd.Equal(u.WindowStart.Add(e.window)) {
			e.logger.Printf("dropping misaligned window for host %s: %s - %s", u.Host, u.WindowStart, u.WindowEnd)
			continue
		}
		key := usageKey{serviceID: svcID, windowStart: u.WindowStart}
		agg := aggregates[key]
		agg.ServiceID = svcID
		agg.WindowStart = u.WindowStart
		agg.WindowEnd = u.WindowEnd
		agg.PrimaryBytes += u.Bytes
		aggregates[key] = agg
	}

	for key, params := range aggregates {
		if params.WindowEnd.IsZero() {
			params.WindowEnd = params.WindowStart.Add(e.window)
		}
		if err := e.queries.UpsertUsageSnapshot(ctx, params); err != nil {
			return fmt.Errorf("persist usage for service %d window %s: %w", key.serviceID, key.windowStart, err)
		}
	}

	e.logger.Printf("ingested %d windows across %d services", len(aggregates), len(domainMap))
	return nil
}

type usageKey struct {
	serviceID   int64
	windowStart time.Time
}

func (e *Engine) loadDomains(ctx context.Context, services []db.Service) (map[int64][]db.ServiceDomain, map[string]int64, error) {
	serviceSet := make(map[int64]struct{}, len(services))
	for _, svc := range services {
		serviceSet[svc.ID] = struct{}{}
	}

	domains, err := e.queries.GetAllServiceDomains(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch domains: %w", err)
	}

	byService := make(map[int64][]db.ServiceDomain)
	hostToService := make(map[string]int64)
	for _, d := range domains {
		if _, ok := serviceSet[d.ServiceID]; !ok {
			continue
		}
		byService[d.ServiceID] = append(byService[d.ServiceID], d)
		hostToService[d.Name] = d.ServiceID
	}
	return byService, hostToService, nil
}
