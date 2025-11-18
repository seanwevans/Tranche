-- Services

-- name: GetActiveServices :many
SELECT *
FROM services
WHERE deleted_at IS NULL
ORDER BY id;

-- name: GetServiceDomains :many
SELECT *
FROM service_domains
WHERE service_id = $1
ORDER BY id;

-- Storm policies/events

-- name: GetStormPoliciesForService :many
SELECT *
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
