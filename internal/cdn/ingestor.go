package cdn

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"tranche/internal/db"
)

type UsageIngestor struct {
	db      *db.Queries
	selectr *Selector
	log     Logger
	window  time.Duration
}

func NewUsageIngestor(dbx *db.Queries, selector *Selector, log Logger, window time.Duration) *UsageIngestor {
	if window <= 0 {
		window = time.Hour
	}
	return &UsageIngestor{db: dbx, selectr: selector, log: log, window: window}
}

func (i *UsageIngestor) RunOnce(ctx context.Context, now time.Time) error {
	windowEnd := now.Truncate(i.window)
	windowStart := windowEnd.Add(-i.window)

	services, err := i.db.GetActiveServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	for _, svc := range services {
		if err := i.ingestService(ctx, svc, windowStart, windowEnd); err != nil {
			if i.log != nil {
				i.log.Printf("usage ingestion for service %d: %v", svc.ID, err)
			}
		}
	}

	return nil
}

func (i *UsageIngestor) ingestService(ctx context.Context, svc db.Service, start, end time.Time) error {
	if _, err := i.db.GetUsageSnapshotForWindow(ctx, db.GetUsageSnapshotForWindowParams{ServiceID: svc.ID, WindowStart: start, WindowEnd: end}); err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("check existing snapshot: %w", err)
	}

	provider, err := i.selectr.ProviderForService(svc)
	if err != nil {
		return err
	}
	primaryBytes, backupBytes, err := provider.FetchUsage(ctx, svc, start, end)
	if err != nil {
		return fmt.Errorf("fetch usage: %w", err)
	}

	if err := i.db.UpsertUsageSnapshot(ctx, db.UpsertUsageSnapshotParams{
		ServiceID:    svc.ID,
		WindowStart:  start,
		WindowEnd:    end,
		PrimaryBytes: primaryBytes,
		BackupBytes:  backupBytes,
	}); err != nil {
		return fmt.Errorf("insert usage snapshot: %w", err)
	}

	if i.log != nil {
		i.log.Printf("recorded usage window %s-%s for service %d (primary=%d backup=%d)", start.Format(time.RFC3339), end.Format(time.RFC3339), svc.ID, primaryBytes, backupBytes)
	}
	return nil
}
