-- Services

-- name: GetActiveServices :many
SELECT id, customer_id, name, primary_cdn, backup_cdn
FROM services
WHERE deleted_at IS NULL
ORDER BY id;

-- name: GetServiceDomains :many
SELECT id, service_id, name
FROM service_domains
WHERE service_id = $1
ORDER BY id;

-- Storm policies/events

-- name: GetStormPoliciesForService :many
SELECT id, service_id, kind, threshold_avail, window_seconds, cooldown_seconds, max_coverage_factor
FROM storm_policies
WHERE service_id = $1
ORDER BY id;

-- name: GetActiveStormsForService :many
SELECT id, service_id, kind
FROM storm_events
WHERE service_id = $1
  AND ended_at IS NULL;

-- name: InsertStormEvent :one
INSERT INTO storm_events (service_id, kind)
VALUES ($1, $2)
RETURNING id, service_id, kind;
