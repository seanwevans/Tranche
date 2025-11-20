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
	"tranche/internal/health"
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

	metrics := observability.NewMetrics(nil, nil)
	metricsAddr := cfg.MetricsAddr
	if metricsAddr == "" {
		metricsAddr = ":9093"
	}

	planner := routing.NewPlanner(queries)
	var dnsProv dns.Provider = dns.NewNoopProvider(logger)
	providerName := "noop"
	providerReady := true
	if cfg.AWSRegion != "" {
		awsCfg := dns.Route53ProviderConfig{
			Region:          cfg.AWSRegion,
			AccessKeyID:     cfg.AWSAccessKey,
			SecretAccessKey: cfg.AWSSecretKey,
			SessionToken:    cfg.AWSSession,
		}
		prov, err := dns.NewRoute53Provider(ctx, logger, awsCfg)
		if err != nil {
			providerReady = false
			logger.Errorf("route53 initialization failed: %v", err)
		} else {
			dnsProv = prov
			providerName = "route53"
		}
	}

	dnsProv = &instrumentedProvider{Provider: dnsProv, metrics: metrics, provider: providerName}

	observability.StartServer(ctx, metricsAddr, metrics, logger, func(ctx context.Context) error {
		if err := health.ReadyCheck(ctx, sqlDB); err != nil {
			return err
		}
		if !providerReady {
			return fmt.Errorf("dns provider not initialized")
		}
		return nil
	})

	reconcile := func() {
		servicesCtx, servicesCancel := context.WithTimeout(ctx, 5*time.Second)
		services, err := queries.GetActiveServices(servicesCtx)
		servicesCancel()
		if err != nil {
			logger.Errorf("GetActiveServices: %v", err)
			return
		}
		for _, s := range services {
			weightsCtx, weightsCancel := context.WithTimeout(ctx, 5*time.Second)
			weights, err := planner.DesiredRouting(weightsCtx, s.ID)
			weightsCancel()
			if err != nil {
				logger.Errorf("DesiredRouting(service=%d): %v", s.ID, err)
				continue
			}
			domainsCtx, domainsCancel := context.WithTimeout(ctx, 5*time.Second)
			domains, err := queries.GetServiceDomains(domainsCtx, s.ID)
			domainsCancel()
			if err != nil {
				logger.Errorf("GetServiceDomains(service=%d): %v", s.ID, err)
				continue
			}
			for _, dom := range domains {
				setWeightsCtx, setWeightsCancel := context.WithTimeout(ctx, 5*time.Second)
				if err := dnsProv.SetWeights(setWeightsCtx, dom.Name, weights.Primary, weights.Backup); err != nil {
					logger.Errorf("SetWeights(%s): %v", dom.Name, err)
				}
				setWeightsCancel()
			}
		}
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	reconcile()

	for {
		select {
		case <-ctx.Done():
			logger.Println("shutting down dns-operator")
			_ = sqlDB.Close()
			return
		case <-ticker.C:
			servicesCtx, servicesCancel := context.WithTimeout(ctx, 5*time.Second)
			services, err := queries.GetActiveServices(servicesCtx)
			servicesCancel()
			if err != nil {
				logger.Errorf("GetActiveServices: %v", err)
				continue
			}
			for _, s := range services {
				weightsCtx, weightsCancel := context.WithTimeout(ctx, 5*time.Second)
				weights, err := planner.DesiredRouting(weightsCtx, s.ID)
				weightsCancel()
				if err != nil {
					logger.Errorf("DesiredRouting(service=%d): %v", s.ID, err)
					continue
				}
				domainsCtx, domainsCancel := context.WithTimeout(ctx, 5*time.Second)
				domains, err := queries.GetServiceDomains(domainsCtx, s.ID)
				domainsCancel()
				if err != nil {
					logger.Errorf("GetServiceDomains(service=%d): %v", s.ID, err)
					continue
				}
				for _, dom := range domains {
					setWeightsCtx, setWeightsCancel := context.WithTimeout(ctx, 5*time.Second)
					if err := dnsProv.SetWeights(setWeightsCtx, dom.Name, weights.Primary, weights.Backup); err != nil {
						logger.Errorf("SetWeights(%s): %v", dom.Name, err)
					}
					setWeightsCancel()
				}
			}
		}
	}
}

type instrumentedProvider struct {
	dns.Provider
	metrics  *observability.Metrics
	provider string
}

func (p *instrumentedProvider) SetWeights(ctx context.Context, domain string, primaryWeight, backupWeight int) error {
	err := p.Provider.SetWeights(ctx, domain, primaryWeight, backupWeight)
	status := "success"
	if err != nil {
		status = "error"
	}
	if p.metrics != nil {
		p.metrics.RecordDNSChange(p.provider, domain, status)
	}
	return err
}
