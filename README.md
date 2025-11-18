# tranche (skeleton)

Early skeleton for the Cloudflare-storm / multi-CDN insurance MVP.

This repo is intentionally small and boring:

- **Postgres** is the source of truth for config, storms, and billing facts.
- **Go** services do:
  - health probing and storm detection
  - routing decisions and DNS updates
  - HTTP control-plane API
  - billing/invoicing batch work

## Quick start

### 1. Postgres

Create a local database:

```bash
createdb tranche
psql tranche < migrations/0001_init.sql
```

Default connection string (can be overridden):

```bash
export PG_DSN="postgres://user:pass@localhost:5432/tranche?sslmode=disable"
```

### 2. Generate real DB code with sqlc

Install [sqlc](https://docs.sqlc.dev/en/latest/):

```bash
# e.g. on macOS
brew install sqlc
```

Generate Go bindings for the queries:

```bash
sqlc generate
```

This will overwrite `internal/db/queries.go` with real code based on:

- `migrations/` (schema)
- `internal/db/queries.sql` (named queries)

### 3. Run the control-plane

```bash
go run ./cmd/control-plane
```

Check that it boots:

```bash
curl http://localhost:8080/healthz
# -> "ok"
curl http://localhost:8080/v1/services
# -> [] (empty JSON array, until you insert rows)
```

### 4. Run the prober and DNS operator (dev mode)

In separate terminals:

```bash
go run ./cmd/prober
go run ./cmd/dns-operator
go run ./cmd/billing-worker
```

Right now they:

- Prober: runs a dummy HTTPS GET against `https://example.com/healthz` for each service (TODO: wire real domains).
- Storm engine: checks in-memory metrics; if availability < threshold, inserts a `storm_events` row.
- DNS operator: reads `storm_events` and calls the **noop** DNS provider (logs intended weight changes).
- Billing worker: logs a stub line on each tick.

### 5. Wiring to real DNS/CDN (next steps)

Replace the noop DNS provider with a real implementation, e.g. Route53:

- Add credentials via environment / config.
- Implement `SetWeights(domain, primary, backup)` to:
  - Look up the hosted zone for the domain.
  - Update weighted records for primary vs backup CNAMEs.

Similarly, add a CDN integration layer under `internal/cdn/` when you’re ready.

## Schema sketch

The initial schema contains:

- `customers` – accounts.
- `services` – a unit of failover (e.g. `app.example.com`).
- `service_domains` – one or more hostnames per service.
- `storm_policies` – thresholds for declaring storms, per service.
- `storm_events` – recorded storms (start/end, kind).

You can extend this with:

- `usage_snapshots` – traffic usage per service/period from CDN logs.
- `invoices` – generated bills with storm-time discounts.

## Notes

- `internal/db/queries.go` is a **placeholder** so the skeleton builds. Run `sqlc generate` to replace it.
- The prober currently targets a hard-coded URL; real deployments should:
  - Probe through Cloudflare and backup CDNs separately.
  - Track metrics per service/domain.
- Nothing in this skeleton is “final” – it’s a starting point to iterate on product logic and infra.
