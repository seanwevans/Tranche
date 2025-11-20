package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/dns"
	"tranche/internal/logging"
	"tranche/internal/observability"
	"tranche/internal/routing"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	cfg := config.Load()
	logger := logging.New("dns-operator")

	sqlDB, queries, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}

	planner := routing.NewPlanner(queries)
	var (
		dnsProv      dns.Provider = dns.NewNoopProvider(logger)
		providerInit bool
	)
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
			providerInit = true
		}
	}

	metrics := observability.NewMetrics("dns_operator")
	readyCheck := func(c context.Context) error {
		if err := db.Ready(c, sqlDB); err != nil {
			return err
		}
		if cfg.AWSRegion != "" && !providerInit {
			return fmt.Errorf("route53 provider not initialized")
		}
		return nil
	}

	observability.Start(ctx, cfg.MetricsAddr, logger, metrics.Registry, readyCheck)

	reconcile := func() {
		servicesCtx, servicesCancel := context.WithTimeout(ctx, 5*time.Second)
		services, err := queries.GetActiveServices(servicesCtx)
		servicesCancel()
		if err != nil {
			logger.Printf("GetActiveServices: %v", err)
			return
		}
		for _, s := range services {
			weightsCtx, weightsCancel := context.WithTimeout(ctx, 5*time.Second)
			weights, err := planner.DesiredRouting(weightsCtx, s.ID)
			weightsCancel()
			if err != nil {
				logger.Printf("DesiredRouting(service=%d): %v", s.ID, err)
				continue
			}
			domainsCtx, domainsCancel := context.WithTimeout(ctx, 5*time.Second)
			domains, err := queries.GetServiceDomains(domainsCtx, s.ID)
			domainsCancel()
			if err != nil {
				logger.Printf("GetServiceDomains(service=%d): %v", s.ID, err)
				continue
			}
			for _, dom := range domains {
				setWeightsCtx, setWeightsCancel := context.WithTimeout(ctx, 5*time.Second)
				if err := dnsProv.SetWeights(setWeightsCtx, dom.Name, weights.Primary, weights.Backup); err != nil {
					metrics.RecordDNSChange(dom.Name, "route53", err)
					logger.Error("route53 weight update failed", "domain", dom.Name, "error", err)
				} else {
					metrics.RecordDNSChange(dom.Name, "route53", nil)
					logger.Info("route53 weights updated", "domain", dom.Name, "primary_weight", weights.Primary, "backup_weight", weights.Backup)
				}
				setWeightsCancel()
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
			servicesCtx, servicesCancel := context.WithTimeout(ctx, 5*time.Second)
			services, err := queries.GetActiveServices(servicesCtx)
			servicesCancel()
			if err != nil {
				logger.Printf("GetActiveServices: %v", err)
				continue
			}
			for _, s := range services {
				weightsCtx, weightsCancel := context.WithTimeout(ctx, 5*time.Second)
				weights, err := planner.DesiredRouting(weightsCtx, s.ID)
				weightsCancel()
				if err != nil {
					logger.Printf("DesiredRouting(service=%d): %v", s.ID, err)
					continue
				}
				domainsCtx, domainsCancel := context.WithTimeout(ctx, 5*time.Second)
				domains, err := queries.GetServiceDomains(domainsCtx, s.ID)
				domainsCancel()
				if err != nil {
					logger.Printf("GetServiceDomains(service=%d): %v", s.ID, err)
					continue
				}
				for _, dom := range domains {
					setWeightsCtx, setWeightsCancel := context.WithTimeout(ctx, 5*time.Second)
					if err := dnsProv.SetWeights(setWeightsCtx, dom.Name, weights.Primary, weights.Backup); err != nil {
						metrics.RecordDNSChange(dom.Name, "route53", err)
						logger.Error("route53 weight update failed", "domain", dom.Name, "error", err)
					} else {
						metrics.RecordDNSChange(dom.Name, "route53", nil)
						logger.Info("route53 weights updated", "domain", dom.Name, "primary_weight", weights.Primary, "backup_weight", weights.Backup)
					}
					setWeightsCancel()
				}
			}
		}
	}
}
