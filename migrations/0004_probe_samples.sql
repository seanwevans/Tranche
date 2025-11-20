-- Store raw probe samples for availability calculations

CREATE TABLE probe_samples (
    id          BIGSERIAL PRIMARY KEY,
    service_id  BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    metrics_key TEXT NOT NULL,
    probed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ok          BOOLEAN NOT NULL,
    latency_ms  INTEGER
);

CREATE INDEX idx_probe_samples_service_time
    ON probe_samples (service_id, probed_at DESC);

CREATE INDEX idx_probe_samples_service_key_time
    ON probe_samples (service_id, metrics_key, probed_at DESC);
