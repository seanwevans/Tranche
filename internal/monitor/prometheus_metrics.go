package monitor

import (
	"context"
	"strconv"
	"time"

	"tranche/internal/observability"
)

// PrometheusMetrics adapts the shared metrics bundle to the Scheduler interface.
type PrometheusMetrics struct {
	metrics *observability.Metrics
}

func NewPrometheusMetrics(m *observability.Metrics) *PrometheusMetrics {
	return &PrometheusMetrics{metrics: m}
}

func (m *PrometheusMetrics) RecordProbe(ctx context.Context, serviceID int64, target string, ok bool, latency time.Duration) error {
	if m == nil || m.metrics == nil {
		return nil
	}
	sid := strconv.FormatInt(serviceID, 10)
	result := "success"
	if !ok {
		result = "failure"
	}
	m.metrics.ProbeResults.WithLabelValues(sid, target, result).Inc()
	m.metrics.ProbeLatency.WithLabelValues(sid, target).Observe(latency.Seconds())
	return nil
}

// MultiMetrics fan-outs probe recording to multiple recorders.
type MultiMetrics struct {
	recorders []MetricsRecorder
}

func NewMultiMetrics(recorders ...MetricsRecorder) *MultiMetrics {
	return &MultiMetrics{recorders: recorders}
}

func (m *MultiMetrics) RecordProbe(ctx context.Context, serviceID int64, target string, ok bool, latency time.Duration) error {
	var firstErr error
	for _, r := range m.recorders {
		if r == nil {
			continue
		}
		if err := r.RecordProbe(ctx, serviceID, target, ok, latency); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
