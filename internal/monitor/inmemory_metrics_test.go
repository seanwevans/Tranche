package monitor

import (
	"context"
	"testing"
	"time"
)

func TestAvailabilityNoSamplesDefaultsToZero(t *testing.T) {
	m := NewInMemoryMetrics()
	got, err := m.Availability(context.Background(), 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected availability 0 for empty samples, got %v", got)
	}
}

func TestAvailabilityUsesConfigurableEmptyDefault(t *testing.T) {
	m := NewInMemoryMetricsWithDefault(0.25)
	got, err := m.Availability(context.Background(), 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.25 {
		t.Fatalf("expected configured empty availability, got %v", got)
	}
}

func TestAvailabilityCleansExpiredTargetsAndReturnsDefault(t *testing.T) {
	m := NewInMemoryMetrics()
	m.samples[1] = map[string][]probeSample{
		"target": {{t: time.Now().Add(-2 * time.Minute), ok: true}},
	}

	got, err := m.Availability(context.Background(), 1, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected availability 0 when samples are expired, got %v", got)
	}
	if _, ok := m.samples[1]; ok {
		t.Fatalf("expected service samples to be cleaned up when all targets expire")
	}
}
