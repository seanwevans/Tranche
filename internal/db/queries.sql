-- name: GetActiveServices :many
SELECT *
FROM services
WHERE deleted_at IS NULL
ORDER BY id;

-- name: GetActiveServicesForCustomer :many
SELECT *
FROM services
WHERE deleted_at IS NULL
  AND customer_id = $1
ORDER BY id;

-- name: GetServiceForCustomer :one
SELECT *
FROM services
WHERE id = $1
  AND customer_id = $2
  AND deleted_at IS NULL;

-- name: InsertService :one
INSERT INTO services (customer_id, name, primary_cdn, backup_cdn)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateService :one
UPDATE services
SET name = $3,
    primary_cdn = $4,
    backup_cdn = $5
WHERE id = $1
  AND customer_id = $2
RETURNING *;

-- name: SoftDeleteService :one
UPDATE services
SET deleted_at = NOW()
WHERE id = $1
  AND customer_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- name: GetServiceDomains :many
SELECT *
FROM service_domains
WHERE service_id = $1
ORDER BY id;

-- name: InsertServiceDomain :one
INSERT INTO service_domains (service_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: DeleteServiceDomain :one
DELETE FROM service_domains
WHERE id = $1
  AND service_id = $2
RETURNING *;

-- name: GetStormPoliciesForService :many
SELECT *
FROM storm_policies
WHERE service_id = $1
ORDER BY id;

-- name: GetStormPolicyForService :one
SELECT *
FROM storm_policies
WHERE id = $1
  AND service_id = $2;

-- name: InsertStormPolicy :one
INSERT INTO storm_policies (
        service_id,
        kind,
        threshold_avail,
        window_seconds,
        cooldown_seconds,
        max_coverage_factor)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateStormPolicy :one
UPDATE storm_policies
SET kind = $3,
    threshold_avail = $4,
    window_seconds = $5,
    cooldown_seconds = $6,
    max_coverage_factor = $7
WHERE id = $1
  AND service_id = $2
RETURNING *;

-- name: DeleteStormPolicy :one
DELETE FROM storm_policies
WHERE id = $1
  AND service_id = $2
RETURNING *;

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
