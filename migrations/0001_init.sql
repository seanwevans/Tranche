-- Initial schema for tranche

CREATE TABLE customers (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE services (
    id           BIGSERIAL PRIMARY KEY,
    customer_id  BIGINT NOT NULL REFERENCES customers(id),
    name         TEXT NOT NULL,
    primary_cdn  TEXT NOT NULL,
    backup_cdn   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE TABLE service_domains (
    id          BIGSERIAL PRIMARY KEY,
    service_id  BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(service_id, name)
);

CREATE TABLE storm_policies (
    id                 BIGSERIAL PRIMARY KEY,
    service_id         BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL,
    threshold_avail    DOUBLE PRECISION NOT NULL DEFAULT 0.90,
    window_seconds     INTEGER NOT NULL DEFAULT 60,
    cooldown_seconds   INTEGER NOT NULL DEFAULT 300,
    max_coverage_factor DOUBLE PRECISION NOT NULL DEFAULT 1.5,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE storm_events (
    id          BIGSERIAL PRIMARY KEY,
    service_id  BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ
);

CREATE INDEX idx_storm_events_service_active
    ON storm_events (service_id)
    WHERE ended_at IS NULL;
