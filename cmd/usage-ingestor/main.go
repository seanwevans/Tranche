package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	cf "tranche/internal/cdn/cloudflare"
	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/logging"
	"tranche/internal/observability"
	"tranche/internal/usageingestor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := config.Load()
	logger := logging.New("usage-ingestor")

	provider := cf.NewClient(cfg.CloudflareAccountID, cfg.CloudflareAPIToken)
	if cfg.CloudflareAccountID == "" || cfg.CloudflareAPIToken == "" {
		logger.Fatal("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN must be set")
	}

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}
	defer sqlDB.Close()

	metrics := observability.NewMetrics("usage-ingestor")
	observability.Start(ctx, cfg.MetricsAddr, logger, metrics.Registry, func(c context.Context) error {
		return db.Ready(c, sqlDB)
	})

	engine := usageingestor.NewEngine(queries, provider, logger, cfg.UsageWindow, cfg.UsageLookback)

	ticker := time.NewTicker(cfg.UsageTick)
	defer ticker.Stop()

	logger.Printf("usage ingestor starting with window %s lookback %s", cfg.UsageWindow, cfg.UsageLookback)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := engine.RunOnce(ctx, time.Now()); err != nil {
				logger.Error("usage ingestion error", "error", err)
			}
		}
	}
}
