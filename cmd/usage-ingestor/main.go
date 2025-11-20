package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/cdn"
	cf "tranche/internal/cdn/cloudflare"
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

	providers := []cdn.Provider{}
	if cfg.Cloudflare.APIToken != "" {
		p, err := cf.NewProvider(cfg.Cloudflare, logger)
		if err != nil {
			logger.Fatalf("init cloudflare provider: %v", err)
		}
		providers = append(providers, p)
	}

	selector, err := cdn.NewSelector(cdn.SelectorConfig{
		DefaultProvider:   cfg.CDNDefaultProvider,
		CustomerOverrides: cfg.CDNCustomerProviders,
		ServiceOverrides:  cfg.CDNServiceProviders,
		Providers:         providers,
	})
	if err != nil {
		logger.Fatalf("init provider selector: %v", err)
	}

	ingestor := cdn.NewUsageIngestor(queries, selector, logger, cfg.UsageWindow)

	ticker := time.NewTicker(cfg.UsageWindow)
	defer ticker.Stop()

	for {
		if err := ingestor.RunOnce(ctx, time.Now()); err != nil {
			logger.Printf("usage ingestion tick error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
