package observability

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ProbeAvailability interface {
	Availability(serviceID int64, window time.Duration) float64
}

// Metrics centralizes Prometheus instrumentation and keeps the in-memory probe
// samples needed for availability calculations.
type Metrics struct {
	registry *prometheus.Registry

	probeSamples ProbeAvailability
	probeResults *prometheus.CounterVec
	probeLatency *prometheus.HistogramVec

	stormEvents *prometheus.CounterVec
	stormActive *prometheus.GaugeVec

	dnsChanges *prometheus.CounterVec

	billingDuration *prometheus.HistogramVec
	billingInvoices *prometheus.CounterVec
}

// NewMetrics builds a metrics container backed by the provided registry. If no
// registry is supplied, a new one is created.
func NewMetrics(reg *prometheus.Registry, availability ProbeAvailability) *Metrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	m := &Metrics{registry: reg, probeSamples: availability}

	m.probeResults = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tranche_probe_results_total",
		Help: "Counts of probe results grouped by service and target",
	}, []string{"service_id", "target", "result"})
	m.probeLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tranche_probe_latency_seconds",
		Help:    "Probe latency distributions",
		Buckets: prometheus.DefBuckets,
	}, []string{"service_id", "target"})

	m.stormEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tranche_storm_events_total",
		Help: "Storm events emitted grouped by service and kind",
	}, []string{"service_id", "kind", "status"})
	m.stormActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tranche_storm_active",
		Help: "Active storm gauges grouped by service and kind",
	}, []string{"service_id", "kind"})

	m.dnsChanges = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tranche_dns_changes_total",
		Help: "DNS weight updates grouped by provider and domain",
	}, []string{"provider", "domain", "status"})

	m.billingDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tranche_billing_run_seconds",
		Help:    "Durations of billing runs",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})
	m.billingInvoices = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tranche_billing_invoices_total",
		Help: "Invoices generated per run",
	}, []string{"status"})

	reg.MustRegister(m.probeResults, m.probeLatency, m.stormEvents, m.stormActive, m.dnsChanges, m.billingDuration, m.billingInvoices)

	return m
}

func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

func (m *Metrics) RecordProbe(serviceID int64, target string, ok bool, latency time.Duration) {
	result := "failure"
	if ok {
		result = "success"
	}
	sid := formatID(serviceID)
	if rec, ok := m.probeSamples.(interface {
		RecordProbe(serviceID int64, target string, ok bool, latency time.Duration)
	}); ok {
		rec.RecordProbe(serviceID, target, ok, latency)
	}
	m.probeResults.WithLabelValues(sid, target, result).Inc()
	m.probeLatency.WithLabelValues(sid, target).Observe(latency.Seconds())
}

func (m *Metrics) Availability(serviceID int64, window time.Duration) float64 {
	if m.probeSamples == nil {
		return 0
	}
	return m.probeSamples.Availability(serviceID, window)
}

func (m *Metrics) RecordStormEvent(serviceID int64, kind, status string, active bool) {
	sid := formatID(serviceID)
	m.stormEvents.WithLabelValues(sid, kind, status).Inc()
	if active {
		m.stormActive.WithLabelValues(sid, kind).Set(1)
	} else {
		m.stormActive.WithLabelValues(sid, kind).Set(0)
	}
}

func (m *Metrics) SetStormActive(serviceID int64, kind string, active bool) {
	sid := formatID(serviceID)
	if active {
		m.stormActive.WithLabelValues(sid, kind).Set(1)
		return
	}
	m.stormActive.WithLabelValues(sid, kind).Set(0)
}

func (m *Metrics) RecordDNSChange(provider, domain, status string) {
	m.dnsChanges.WithLabelValues(provider, domain, status).Inc()
}

func (m *Metrics) ObserveBillingRun(duration time.Duration, invoices int, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	m.billingDuration.WithLabelValues(status).Observe(duration.Seconds())
	m.billingInvoices.WithLabelValues(status).Add(float64(invoices))
}

func formatID(id int64) string {
	return fmt.Sprintf("%d", id)
}
