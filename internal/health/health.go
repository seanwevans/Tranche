package health

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"tranche/internal/db"
)

// ReadyCheck validates DB connectivity and migration state.
func ReadyCheck(ctx context.Context, conn *sql.DB) error {
	if conn == nil {
		return fmt.Errorf("database connection not initialized")
	}
	pingCtx, cancelPing := context.WithTimeout(ctx, 2*time.Second)
	defer cancelPing()
	if err := conn.PingContext(pingCtx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	if err := db.CheckMigrations(ctx, conn); err != nil {
		return fmt.Errorf("migration status: %w", err)
	}
	return nil
}
