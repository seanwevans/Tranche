package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/dns"
	"tranche/internal/logging"
	"tranche/internal/routing"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	cfg := config.Load()
	logger := logging.New()

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}

	planner := routing.NewPlanner(queries)
	var dnsProv dns.Provider = dns.NewNoopProvider(logger)
	if cfg.AWSRegion != "" {
		awsCfg := dns.Route53ProviderConfig{
			Region:          cfg.AWSRegion,
			AccessKeyID:     cfg.AWSAccessKey,
			SecretAccessKey: cfg.AWSSecretKey,
			SessionToken:    cfg.AWSSession,
		}
		prov, err := dns.NewRoute53Provider(ctx, logger, awsCfg)
		if err != nil {
			logger.Printf("failed to init Route53 provider, falling back to noop: %v", err)
		} else {
			dnsProv = prov
		}
	}

	reconcile := func() {
		services, err := queries.GetActiveServices(ctx)
		if err != nil {
			logger.Printf("GetActiveServices: %v", err)
			return
		}
		for _, s := range services {
			weights, err := planner.DesiredRouting(ctx, s.ID)
			if err != nil {
				logger.Printf("DesiredRouting(service=%d): %v", s.ID, err)
				continue
			}
			domains, err := queries.GetServiceDomains(ctx, s.ID)
			if err != nil {
				logger.Printf("GetServiceDomains(service=%d): %v", s.ID, err)
				continue
			}
			for _, dom := range domains {
				if err := dnsProv.SetWeights(ctx, dom.Name, weights.Primary, weights.Backup); err != nil {
					logger.Printf("SetWeights(%s): %v", dom.Name, err)
				}
			}
		}
	}

	ticker := time.NewTicker(15 * time.Second)

	reconcile()

	for {
		select {
		case <-ctx.Done():
			logger.Println("shutting down dns-operator")
			ticker.Stop()
			_ = sqlDB.Close()
			return
		case <-ticker.C:
			services, err := queries.GetActiveServices(ctx)
			if err != nil {
				logger.Printf("GetActiveServices: %v", err)
				continue
			}
			for _, s := range services {
				weights, err := planner.DesiredRouting(ctx, s.ID)
				if err != nil {
					logger.Printf("DesiredRouting(service=%d): %v", s.ID, err)
					continue
				}
				domains, err := queries.GetServiceDomains(ctx, s.ID)
				if err != nil {
					logger.Printf("GetServiceDomains(service=%d): %v", s.ID, err)
					continue
				}
				for _, dom := range domains {
					if err := dnsProv.SetWeights(ctx, dom.Name, weights.Primary, weights.Backup); err != nil {
						logger.Printf("SetWeights(%s): %v", dom.Name, err)
					}
				}
			}
		}
	}
}
