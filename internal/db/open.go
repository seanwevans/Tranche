package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"tranche/migrations"
)

func Open(ctx context.Context, dsn string) (*sql.DB, *Queries, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := runMigrations(ctx, conn); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, New(conn), nil
}

const migrationLockID int64 = 0x7472616e636865 // 'tranche' in hex.

func runMigrations(ctx context.Context, conn *sql.DB) (err error) {
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx := context.WithoutCancel(ctx)
		if _, unlockErr := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("release migration lock: %w", unlockErr)
		}
	}()

	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}
	entries, err := migrationVersions()
	if err != nil {
		return err
	}
	for _, version := range entries {
		if _, ok := applied[version]; ok {
			continue
		}
		contents, err := migrations.Files.ReadFile(version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if err := applyMigration(ctx, conn, version, string(contents)); err != nil {
			return err
		}
	}
	return nil
}

// CheckMigrations verifies that all embedded migrations are applied without
// mutating the database. The advisory lock is intentionally skipped to avoid
// blocking readiness probes.
func CheckMigrations(ctx context.Context, conn *sql.DB) error {
	if err := ensureMigrationsTable(ctx, conn); err != nil {
		return err
	}
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}
	versions, err := migrationVersions()
	if err != nil {
		return err
	}
	for _, v := range versions {
		if _, ok := applied[v]; !ok {
			return fmt.Errorf("pending migration: %s", v)
		}
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, conn *sql.DB) error {
	const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
                version TEXT PRIMARY KEY,
                applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`
	if _, err := conn.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *sql.DB) (map[string]struct{}, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]struct{})
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	return applied, nil
}

func migrationVersions() ([]string, error) {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versions = append(versions, entry.Name())
	}
	sort.Strings(versions)
	return versions, nil
}

func applyMigration(ctx context.Context, conn *sql.DB, version, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}
