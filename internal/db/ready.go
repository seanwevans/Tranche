package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"tranche/migrations"
)

// Ready validates database connectivity and ensures all embedded migrations are present.
func Ready(ctx context.Context, conn *sql.DB) error {
	if conn == nil {
		return fmt.Errorf("db connection is nil")
	}
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
                version TEXT PRIMARY KEY,
                applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`
	if _, err := conn.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]struct{})
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("scan migration version: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	expected := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		expected = append(expected, entry.Name())
	}
	sort.Strings(expected)
	missing := make([]string, 0)
	for _, name := range expected {
		if _, ok := applied[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("pending migrations: %s", strings.Join(missing, ","))
	}
	return nil
}
