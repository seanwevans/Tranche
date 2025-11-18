package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tranche/internal/config"
	"tranche/internal/db"
	"tranche/internal/httpapi"
	"tranche/internal/logging"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := config.Load()
	logger := logging.New()

	dbConn, err := db.Open(ctx, cfg.PGDSN)
	if err != nil {
		logger.Fatalf("opening db: %v", err)
	}
	defer dbConn.Close()

	api := httpapi.NewServer(logger, dbConn)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.Router(),
	}

	go func() {
		logger.Printf("control-plane listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Println("shutting down control-plane")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(shutdownCtx)

	_ = os.Stdout.Sync()
}
