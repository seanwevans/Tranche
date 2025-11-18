-- name: GetActiveServices :many
SELECT id, customer_id, name, primary_cdn, backup_cdn, created_at, deleted_at
FROM services
WHERE deleted_at IS NULL
ORDER BY id;

-- name: GetServiceDomains :many
SELECT id, service_id, name, created_at
FROM service_domains
WHERE service_id = $1
ORDER BY id;

-- name: GetStormPoliciesForService :many
SELECT id, service_id, kind, threshold_avail, window_seconds, cooldown_seconds, max_coverage_factor, created_at
FROM storm_policies
WHERE service_id = $1
ORDER BY id;

-- name: GetActiveStormsForService :many
SELECT id, service_id, kind, started_at, ended_at
FROM storm_events
WHERE service_id = $1
  AND ended_at IS NULL;

-- name: GetActiveStormForPolicy :one
SELECT id, service_id, kind, started_at, ended_at
FROM storm_events
WHERE service_id = $1
  AND kind = $2
  AND ended_at IS NULL
ORDER BY started_at DESC
LIMIT 1;

-- name: GetLastStormEvent :one
SELECT id, service_id, kind, started_at, ended_at
FROM storm_events
WHERE service_id = $1
  AND kind = $2
ORDER BY started_at DESC
LIMIT 1;

-- name: InsertStormEvent :one
INSERT INTO storm_events (service_id, kind)
VALUES ($1, $2)
RETURNING id, service_id, kind, started_at, ended_at;

-- name: MarkStormEventResolved :one
UPDATE storm_events
SET ended_at = $2
WHERE id = $1
RETURNING id, service_id, kind, started_at, ended_at;
