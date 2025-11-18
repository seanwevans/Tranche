# Tranche

Early skeleton for the Cloudflare-storm / multi-CDN insurance MVP.

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
- Billing worker: ingests unbilled `usage_snapshots`, joins active `storm_events`, and persists invoices + line items while logging each invoice ID for observability.

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

- `usage_snapshots` – traffic usage per service/period from CDN logs (now part of the default schema).
- `invoices` / `invoice_line_items` – generated bills that apply storm-time discounts.

## Billing & invoicing flow

The billing worker polls once a minute and executes a full invoicing run:

1. Pull the newest unbilled `usage_snapshots` whose `window_end` falls within the configured billing period.
2. For each snapshot/service, fetch overlapping `storm_events` and compute a coverage ratio that is capped by `storm_policies.max_coverage_factor`.
3. Calculate line-item charges using the configured rate (cents/GB) and discount rate, apply the policy coverage factor, then insert invoice headers + `invoice_line_items` rows.
4. Update each snapshot with the generated invoice ID so the worker never double bills and log every invoice ID for quick observability.

Environment knobs (override via env vars) let you tune the worker without code changes:

| Env var | Default | Description |
| --- | --- | --- |
| `BILLING_PERIOD` | `24h` | Look-back window for the RunOnce query and the period recorded on each invoice. |
| `BILLING_RATE_CENTS_PER_GB` | `12` | Base rate applied to both primary + backup bytes within a snapshot. |
| `BILLING_DISCOUNT_RATE` | `0.5` | Multiplier applied to backup usage when storms overlap the billing window (coverage factors can increase the effective discount up to the policy cap). |

Usage ingestion is intentionally decoupled from billing – populate `usage_snapshots` from CDN logs or metering pipelines, then let the worker mint invoices in the same database transaction that tags the snapshots as billed.

## Notes

- `internal/db/queries.go` is a **placeholder** so the skeleton builds. Run `sqlc generate` to replace it.
- The prober currently targets a hard-coded URL; real deployments should:
  - Probe through Cloudflare and backup CDNs separately.
  - Track metrics per service/domain.
- Nothing in this skeleton is “final” – it’s a starting point to iterate on product logic and infra.
