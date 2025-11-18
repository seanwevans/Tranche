// NOTE: This file is a placeholder so `go build` works out of the box.
// Replace it by running `sqlc generate`, which will overwrite this file
// using `internal/db/queries.sql` and `migrations/`.

package db

import (
        "context"
        "database/sql"
        "time"
)

type Service struct {
        ID         int64  `json:"id"`
        CustomerID int64  `json:"customer_id"`
        Name       string `json:"name"`
        PrimaryCdn string `json:"primary_cdn"`
        BackupCdn  string `json:"backup_cdn"`
}

type ServiceDomain struct {
        ID        int64  `json:"id"`
        ServiceID int64  `json:"service_id"`
        Name      string `json:"name"`
}

type StormPolicy struct {
        ID                int64   `json:"id"`
        ServiceID         int64   `json:"service_id"`
        Kind              string  `json:"kind"`
        ThresholdAvail    float64 `json:"threshold_avail"`
        WindowSeconds     int32   `json:"window_seconds"`
        CooldownSeconds   int32   `json:"cooldown_seconds"`
        MaxCoverageFactor float64 `json:"max_coverage_factor"`
}

type StormEvent struct {
        ID        int64        `json:"id"`
        ServiceID int64        `json:"service_id"`
        Kind      string       `json:"kind"`
        StartedAt time.Time    `json:"started_at"`
        EndedAt   sql.NullTime `json:"ended_at"`
}

type Queries struct {
        db *sql.DB
}

func New(db *sql.DB) *Queries {
        return &Queries{db: db}
}

func (q *Queries) GetActiveServices(ctx context.Context) ([]Service, error) {
        // TODO: replaced by sqlc
        return []Service{}, nil
}

func (q *Queries) GetServiceDomains(ctx context.Context, serviceID int64) ([]ServiceDomain, error) {
        // TODO: replaced by sqlc
        return []ServiceDomain{}, nil
}

func (q *Queries) GetStormPoliciesForService(ctx context.Context, serviceID int64) ([]StormPolicy, error) {
        // TODO: replaced by sqlc
        return []StormPolicy{}, nil
}

func (q *Queries) GetActiveStormsForService(ctx context.Context, serviceID int64) ([]StormEvent, error) {
        // TODO: replaced by sqlc
        return []StormEvent{}, nil
}

type GetActiveStormForPolicyParams struct {
        ServiceID int64
        Kind      string
}

func (q *Queries) GetActiveStormForPolicy(ctx context.Context, arg GetActiveStormForPolicyParams) (StormEvent, error) {
        // TODO: replaced by sqlc
        return StormEvent{}, sql.ErrNoRows
}

type GetLastStormEventParams struct {
        ServiceID int64
        Kind      string
}

func (q *Queries) GetLastStormEvent(ctx context.Context, arg GetLastStormEventParams) (StormEvent, error) {
        // TODO: replaced by sqlc
        return StormEvent{}, sql.ErrNoRows
}

type InsertStormEventParams struct {
        ServiceID int64
        Kind      string
}

func (q *Queries) InsertStormEvent(ctx context.Context, arg InsertStormEventParams) (StormEvent, error) {
        // TODO: replaced by sqlc
        return StormEvent{}, nil
}

type MarkStormEventResolvedParams struct {
        ID      int64
        EndedAt time.Time
}

func (q *Queries) MarkStormEventResolved(ctx context.Context, arg MarkStormEventResolvedParams) (StormEvent, error) {
        // TODO: replaced by sqlc
        return StormEvent{}, nil
}
