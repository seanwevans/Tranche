package billing

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"tranche/internal/db"
)

type Logger interface {
	Printf(string, ...any)
}

type Metrics interface {
	ObserveBillingRun(duration time.Duration, invoices int, err error)
}

type Config struct {
	Period         time.Duration
	RateCentsPerGB int64
	DiscountRate   float64
}

type Engine struct {
	db      *db.Queries
	log     Logger
	cfg     Config
	metrics Metrics
}

type coverageQuerier interface {
	GetMaxCoverageFactorForService(context.Context, int64) (float64, error)
}

func NewEngine(dbx *db.Queries, log Logger, cfg Config, metrics Metrics) *Engine {
	if cfg.Period <= 0 {
		cfg.Period = 24 * time.Hour
	}
	if cfg.RateCentsPerGB <= 0 {
		cfg.RateCentsPerGB = 12
	}
	if cfg.DiscountRate < 0 {
		cfg.DiscountRate = 0
	}
	return &Engine{db: dbx, log: log, cfg: cfg, metrics: metrics}
}

func (e *Engine) RunOnce(ctx context.Context, now time.Time) (err error) {
	since := now.Add(-e.cfg.Period)
	start := time.Now()
	invoicesEmitted := 0
	defer func() {
		if e.metrics != nil {
			e.metrics.ObserveBillingRun(time.Since(start), invoicesEmitted, err)
		}
	}()

	qtx, tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invoice transaction: %w", err)
	}
	defer tx.Rollback()

	snapshots, err := qtx.LockUnbilledUsageSnapshots(ctx, db.LockUnbilledUsageSnapshotsParams{
		WindowEnd:   now,
		WindowStart: since,
	})
	if err != nil {
		return fmt.Errorf("list unbilled snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		e.log.Printf("billing run at %s: no usage in window", now.Format(time.RFC3339))
		return nil
	}

	coverageCache := make(map[int64]float64)
	invoices := make(map[int64]*invoiceBuild)

	for _, snap := range snapshots {
		storms, err := qtx.GetStormEventsForWindow(ctx, db.GetStormEventsForWindowParams{
			ServiceID:   snap.ServiceID,
			WindowEnd:   snap.WindowEnd,
			WindowStart: sql.NullTime{Time: snap.WindowStart, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("storms for service %d: %w", snap.ServiceID, err)
		}
		maxCoverage, err := e.maxCoverageFactor(ctx, qtx, coverageCache, snap.ServiceID)
		if err != nil {
			return err
		}
		coverage := coverageRatio(snap.WindowStart, snap.WindowEnd, storms) * maxCoverage
		if coverage > maxCoverage {
			coverage = maxCoverage
		}

		lineSubtotal := e.chargeForBytes(snap.PrimaryBytes) + e.chargeForBytes(snap.BackupBytes)
		backupCharge := e.chargeForBytes(snap.BackupBytes)
		discount := int64(math.Round(float64(backupCharge) * e.cfg.DiscountRate * coverage))
		if discount > lineSubtotal {
			discount = lineSubtotal
		}
		lineTotal := lineSubtotal - discount

		inv := invoices[snap.CustomerID]
		if inv == nil {
			inv = &invoiceBuild{
				customerID:  snap.CustomerID,
				periodStart: snap.WindowStart,
				periodEnd:   snap.WindowEnd,
			}
			invoices[snap.CustomerID] = inv
		}
		if snap.WindowStart.Before(inv.periodStart) {
			inv.periodStart = snap.WindowStart
		}
		if snap.WindowEnd.After(inv.periodEnd) {
			inv.periodEnd = snap.WindowEnd
		}

		inv.subtotal += lineSubtotal
		inv.discount += discount
		inv.total += lineTotal
		inv.snapshotIDs = append(inv.snapshotIDs, snap.ID)
		inv.items = append(inv.items, lineItem{
			ServiceID:      snap.ServiceID,
			WindowStart:    snap.WindowStart,
			WindowEnd:      snap.WindowEnd,
			PrimaryBytes:   snap.PrimaryBytes,
			BackupBytes:    snap.BackupBytes,
			CoverageFactor: coverage,
			AmountCents:    lineSubtotal,
			DiscountCents:  discount,
		})
	}

	logs := make([]string, 0, len(invoices))

	for _, inv := range invoices {
		sort.Slice(inv.items, func(i, j int) bool {
			return inv.items[i].WindowStart.Before(inv.items[j].WindowStart)
		})
		invoice, err := qtx.InsertInvoice(ctx, db.InsertInvoiceParams{
			CustomerID:    inv.customerID,
			PeriodStart:   inv.periodStart,
			PeriodEnd:     inv.periodEnd,
			SubtotalCents: inv.subtotal,
			DiscountCents: inv.discount,
			TotalCents:    inv.total,
		})
		if err != nil {
			return fmt.Errorf("insert invoice: %w", err)
		}
		for _, item := range inv.items {
			_, err := qtx.InsertInvoiceLineItem(ctx, db.InsertInvoiceLineItemParams{
				InvoiceID:      invoice.ID,
				ServiceID:      item.ServiceID,
				WindowStart:    item.WindowStart,
				WindowEnd:      item.WindowEnd,
				PrimaryBytes:   item.PrimaryBytes,
				BackupBytes:    item.BackupBytes,
				CoverageFactor: item.CoverageFactor,
				AmountCents:    item.AmountCents,
				DiscountCents:  item.DiscountCents,
			})
			if err != nil {
				return fmt.Errorf("insert line item: %w", err)
			}
		}
		for _, snapID := range inv.snapshotIDs {
			if err := qtx.MarkUsageSnapshotInvoiced(ctx, db.MarkUsageSnapshotInvoicedParams{
				InvoiceID: sql.NullInt64{Int64: invoice.ID, Valid: true},
				ID:        snapID,
			}); err != nil {
				return fmt.Errorf("mark snapshot %d invoiced: %w", snapID, err)
			}
		}
		logs = append(logs, fmt.Sprintf("generated invoice %d for customer %d (line_items=%d total_cents=%d)", invoice.ID, invoice.CustomerID, len(inv.items), invoice.TotalCents))
		invoicesEmitted++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invoice batch: %w", err)
	}

	for _, msg := range logs {
		e.log.Printf(msg)
	}

	return nil
}

func (e *Engine) maxCoverageFactor(ctx context.Context, q coverageQuerier, cache map[int64]float64, serviceID int64) (float64, error) {
	if v, ok := cache[serviceID]; ok {
		return v, nil
	}
	factor, err := q.GetMaxCoverageFactorForService(ctx, serviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			cache[serviceID] = 1
			return 1, nil
		}
		return 0, fmt.Errorf("coverage factor for service %d: %w", serviceID, err)
	}
	cache[serviceID] = factor
	return factor, nil
}

func (e *Engine) chargeForBytes(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return int64(math.Round(gb * float64(e.cfg.RateCentsPerGB)))
}

func coverageRatio(windowStart, windowEnd time.Time, storms []db.StormEvent) float64 {
	duration := windowEnd.Sub(windowStart).Seconds()
	if duration <= 0 {
		return 0
	}
	intervals := make([]timeRange, 0, len(storms))
	for _, storm := range storms {
		start := storm.StartedAt
		if start.Before(windowStart) {
			start = windowStart
		}
		end := windowEnd
		if storm.EndedAt.Valid && storm.EndedAt.Time.Before(end) {
			end = storm.EndedAt.Time
		}
		if end.Before(start) {
			continue
		}
		intervals = append(intervals, timeRange{start: start, end: end})
	}
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})
	merged := intervals[:1]
	for _, iv := range intervals[1:] {
		last := &merged[len(merged)-1]
		if iv.start.After(last.end) {
			merged = append(merged, iv)
			continue
		}
		if iv.end.After(last.end) {
			last.end = iv.end
		}
	}
	covered := 0.0
	for _, iv := range merged {
		covered += iv.end.Sub(iv.start).Seconds()
	}
	if covered > duration {
		covered = duration
	}
	return covered / duration
}

type timeRange struct {
	start time.Time
	end   time.Time
}

type invoiceBuild struct {
	customerID  int64
	periodStart time.Time
	periodEnd   time.Time
	subtotal    int64
	discount    int64
	total       int64
	snapshotIDs []int64
	items       []lineItem
}

type lineItem struct {
	ServiceID      int64
	WindowStart    time.Time
	WindowEnd      time.Time
	PrimaryBytes   int64
	BackupBytes    int64
	CoverageFactor float64
	AmountCents    int64
	DiscountCents  int64
}
