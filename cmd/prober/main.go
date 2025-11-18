package main

import (
	"context"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/logging"
	"tranche/internal/monitor"
	"tranche/internal/storm"
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

	metrics := monitor.NewInMemoryMetrics()
	mv := monitor.NewMetricsView(metrics)
	stormEng := storm.NewEngine(dbConn, mv, logger)

	probeSched := monitor.NewScheduler(dbConn, metrics, logger)

	go probeSched.Run(ctx)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := stormEng.Tick(ctx); err != nil {
				logger.Printf("storm tick error: %v", err)
			}
		}
	}
}
