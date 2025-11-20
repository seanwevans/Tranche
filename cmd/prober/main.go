package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/logging"
	"tranche/internal/monitor"
	"tranche/internal/observability"
	"tranche/internal/storm"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	logger := logging.New("prober")

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}

	metrics := observability.NewMetrics("prober")
	probeRecorder := monitor.NewMultiMetrics(
		monitor.NewPostgresMetrics(queries),
		monitor.NewPrometheusMetrics(metrics),
	)
	stormEng := storm.NewEngine(queries, monitor.NewPostgresMetrics(queries), logger).WithMetrics(metrics)

	observability.Start(ctx, cfg.MetricsAddr, logger, metrics.Registry, func(c context.Context) error {
		return db.Ready(c, sqlDB)
	})

	probeSched := monitor.NewScheduler(queries, probeRecorder, logger, monitor.ProbeConfig{
		Path:    cfg.ProbePath,
		Timeout: cfg.ProbeTimeout,
	})

	go probeSched.Run(ctx)

	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			if err := sqlDB.Close(); err != nil {
				logger.Printf("closing db: %v", err)
			}
			return
		case <-ticker.C:
			if err := stormEng.Tick(ctx); err != nil {
				logger.Printf("storm tick error: %v", err)
			}
		}
	}
}
