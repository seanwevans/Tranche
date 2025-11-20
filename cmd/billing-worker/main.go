package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/billing"
	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/logging"
	"tranche/internal/observability"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	logger := logging.New("billing-worker")

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}
	defer sqlDB.Close()

	metrics := observability.NewMetrics("billing")
	observability.Start(ctx, cfg.MetricsAddr, logger, metrics.Registry, func(c context.Context) error {
		return db.Ready(c, sqlDB)
	})

	engine := billing.NewEngine(queries, logger, metrics, billing.Config{
		Period:         cfg.BillingPeriod,
		RateCentsPerGB: cfg.BillingRateCentsPerGB,
		DiscountRate:   cfg.BillingDiscountRate,
	})

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()
			if err := engine.RunOnce(ctx, start); err != nil {
				metrics.RecordBillingRun(time.Since(start), 0, err)
				logger.Error("billing run error", "error", err)
			}
		}
	}
}
