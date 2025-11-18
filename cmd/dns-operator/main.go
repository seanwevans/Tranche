package main

import (
	"context"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/dns"
	"tranche/internal/logging"
	"tranche/internal/routing"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	logger := logging.New()

	dbConn, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}
	defer dbConn.Close()

	planner := routing.NewPlanner(dbConn)
	dnsProv := dns.NewNoopProvider(logger) // replace with Route53/NS1 impl

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			services, err := dbConn.GetActiveServices(ctx)
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
				domains, err := dbConn.GetServiceDomains(ctx, s.ID)
				if err != nil {
					logger.Printf("GetServiceDomains(service=%d): %v", s.ID, err)
					continue
				}
				for _, dom := range domains {
					if err := dnsProv.SetWeights(dom.Name, weights.Primary, weights.Backup); err != nil {
						logger.Printf("SetWeights(%s): %v", dom.Name, err)
					}
				}
			}
		}
	}
}
