package monitor

import (
	"testing"
	"time"
)

func TestAvailabilityNoSamplesDefaultsToZero(t *testing.T) {
	m := NewInMemoryMetrics()
	if got := m.Availability(1, time.Minute); got != 0 {
		t.Fatalf("expected availability 0 for empty samples, got %v", got)
	}
}

func TestAvailabilityUsesConfigurableEmptyDefault(t *testing.T) {
	m := NewInMemoryMetricsWithDefault(0.25)
	if got := m.Availability(1, time.Minute); got != 0.25 {
		t.Fatalf("expected configured empty availability, got %v", got)
	}
}

func TestAvailabilityCleansExpiredTargetsAndReturnsDefault(t *testing.T) {
	m := NewInMemoryMetrics()
	m.samples[1] = map[string][]probeSample{
		"target": {{t: time.Now().Add(-2 * time.Minute), ok: true}},
	}

	if got := m.Availability(1, time.Minute); got != 0 {
		t.Fatalf("expected availability 0 when samples are expired, got %v", got)
	}
	if _, ok := m.samples[1]; ok {
		t.Fatalf("expected service samples to be cleaned up when all targets expire")
	}
}
