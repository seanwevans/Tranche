package observability

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics bundles the Prometheus registry and strongly typed application metrics.
type Metrics struct {
	Registry *prometheus.Registry

	ProbeResults *prometheus.CounterVec
	ProbeLatency *prometheus.HistogramVec

	StormEvents *prometheus.CounterVec
	StormActive *prometheus.GaugeVec

	DNSChanges *prometheus.CounterVec

	BillingRunDuration prometheus.Histogram
	BillingInvoices    prometheus.Counter
	BillingErrors      prometheus.Counter
}

// NewMetrics constructs a registry with the collectors needed by the services.
func NewMetrics(service string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	reg.MustRegister(prometheus.NewGoCollector())

	m := &Metrics{Registry: reg}

	m.ProbeResults = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "probe_results_total",
		Help:      "Number of probe results by target and outcome.",
	}, []string{"service_id", "target", "result"})
	m.ProbeLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "probe_latency_seconds",
		Help:      "Distribution of probe latencies by target.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"service_id", "target"})

	m.StormEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "storm_events_total",
		Help:      "Storm lifecycle events.",
	}, []string{"service_id", "kind", "phase"})
	m.StormActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "storm_active",
		Help:      "Indicator for active storms by service and kind.",
	}, []string{"service_id", "kind"})

	m.DNSChanges = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "dns_changes_total",
		Help:      "DNS provider changes and error counts.",
	}, []string{"domain", "provider", "outcome"})

	m.BillingRunDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "billing_run_duration_seconds",
		Help:      "Billing run durations.",
		Buckets:   prometheus.DefBuckets,
	})
	m.BillingInvoices = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "billing_invoices_total",
		Help:      "Invoices generated across billing runs.",
	})
	m.BillingErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "tranche",
		Subsystem: service,
		Name:      "billing_errors_total",
		Help:      "Billing run errors encountered.",
	})

	reg.MustRegister(
		m.ProbeResults,
		m.ProbeLatency,
		m.StormEvents,
		m.StormActive,
		m.DNSChanges,
		m.BillingRunDuration,
		m.BillingInvoices,
		m.BillingErrors,
	)

	return m
}

// RecordBillingRun captures metrics for a billing attempt.
func (m *Metrics) RecordBillingRun(duration time.Duration, invoices int, err error) {
	if m == nil {
		return
	}
	m.BillingRunDuration.Observe(duration.Seconds())
	if invoices > 0 {
		m.BillingInvoices.Add(float64(invoices))
	}
	if err != nil {
		m.BillingErrors.Inc()
	}
}

// RecordDNSChange records a DNS reconciliation attempt and whether it succeeded.
func (m *Metrics) RecordDNSChange(domain, provider string, err error) {
	if m == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	m.DNSChanges.WithLabelValues(domain, provider, outcome).Inc()
}

// RecordStorm logs lifecycle transitions for storm events.
func (m *Metrics) RecordStorm(serviceID int64, kind, phase string, active bool) {
	if m == nil {
		return
	}
	sid := strconv.FormatInt(serviceID, 10)
	m.StormEvents.WithLabelValues(sid, kind, phase).Inc()
	if active {
		m.StormActive.WithLabelValues(sid, kind).Set(1)
	} else {
		m.StormActive.WithLabelValues(sid, kind).Set(0)
	}
}

// RecordStormEvent is compatible with the storm.Engine metrics interface.
func (m *Metrics) RecordStormEvent(serviceID int64, kind, phase string) {
	m.RecordStorm(serviceID, kind, phase, phase == "started")
}

// SetStormActive updates the active gauge for a storm kind.
func (m *Metrics) SetStormActive(serviceID int64, kind string, active bool) {
	m.RecordStorm(serviceID, kind, "active", active)
}
