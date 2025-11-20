package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/health"
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

	probeSamples := monitor.NewInMemoryMetrics()
	metrics := observability.NewMetrics(nil, probeSamples)
	mv := monitor.NewMetricsView(metrics)
	metricsAddr := cfg.MetricsAddr
	if metricsAddr == "" {
		metricsAddr = ":9092"
	}
	observability.StartServer(ctx, metricsAddr, metrics, logger, func(ctx context.Context) error {
		return health.ReadyCheck(ctx, sqlDB)
	})

	stormEng := storm.NewEngine(queries, mv, metrics, logger)

	probeSched := monitor.NewScheduler(queries, metrics, logger, monitor.ProbeConfig{
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
