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
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	logger := logging.New()

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}
	defer sqlDB.Close()

	engine := billing.NewEngine(queries, logger, billing.Config{
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
			if err := engine.RunOnce(ctx, time.Now()); err != nil {
				logger.Printf("billing run error: %v", err)
			}
		}
	}
}
