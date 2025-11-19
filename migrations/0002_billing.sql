-- Billing tables for usage snapshots and invoices

CREATE TABLE invoices (
    id              BIGSERIAL PRIMARY KEY,
    customer_id     BIGINT NOT NULL REFERENCES customers(id),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    subtotal_cents  BIGINT NOT NULL,
    discount_cents  BIGINT NOT NULL,
    total_cents     BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE invoice_line_items (
    id              BIGSERIAL PRIMARY KEY,
    invoice_id      BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    service_id      BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    window_start    TIMESTAMPTZ NOT NULL,
    window_end      TIMESTAMPTZ NOT NULL,
    primary_bytes   BIGINT NOT NULL DEFAULT 0,
    backup_bytes    BIGINT NOT NULL DEFAULT 0,
    coverage_factor DOUBLE PRECISION NOT NULL DEFAULT 0,
    amount_cents    BIGINT NOT NULL,
    discount_cents  BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE usage_snapshots (
    id            BIGSERIAL PRIMARY KEY,
    service_id    BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    window_start  TIMESTAMPTZ NOT NULL,
    window_end    TIMESTAMPTZ NOT NULL,
    primary_bytes BIGINT NOT NULL DEFAULT 0,
    backup_bytes  BIGINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    invoice_id    BIGINT REFERENCES invoices(id),
    CONSTRAINT usage_snapshot_window CHECK (window_end > window_start)
);

CREATE UNIQUE INDEX idx_usage_snapshots_service_window
    ON usage_snapshots (service_id, window_start, window_end);

CREATE INDEX idx_usage_snapshots_unbilled
    ON usage_snapshots (service_id, window_end)
    WHERE invoice_id IS NULL;
