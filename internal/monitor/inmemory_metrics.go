package monitor

import (
	"context"
	"sync"
	"time"
)

type probeSample struct {
	t  time.Time
	ok bool
}

type InMemoryMetrics struct {
	mu                sync.Mutex
	samples           map[int64]map[string][]probeSample
	emptyAvailability float64
}

func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		samples:           make(map[int64]map[string][]probeSample),
		emptyAvailability: 0,
	}
}

func NewInMemoryMetricsWithDefault(emptyAvailability float64) *InMemoryMetrics {
	return &InMemoryMetrics{
		samples:           make(map[int64]map[string][]probeSample),
		emptyAvailability: emptyAvailability,
	}
}

func (m *InMemoryMetrics) RecordProbe(_ context.Context, serviceID int64, target string, ok bool, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.samples[serviceID]; !ok {
		m.samples[serviceID] = make(map[string][]probeSample)
	}
	m.samples[serviceID][target] = append(m.samples[serviceID][target], probeSample{
		t:  time.Now(),
		ok: ok,
	})
	return nil
}

func (m *InMemoryMetrics) Availability(_ context.Context, serviceID int64, window time.Duration) (float64, error) {
	cutoff := time.Now().Add(-window)
	m.mu.Lock()
	defer m.mu.Unlock()

	targets := m.samples[serviceID]
	if len(targets) == 0 {
		return m.emptyAvailability, nil
	}

	total := 0
	okCount := 0
	emptyTargets := make([]string, 0)
	for target, samples := range targets {
		idx := len(samples)
		for i, sample := range samples {
			if sample.t.After(cutoff) {
				idx = i
				break
			}
		}
		samples = samples[idx:]
		if len(samples) == 0 {
			emptyTargets = append(emptyTargets, target)
			continue
		}
		targets[target] = samples
		total += len(samples)
		for _, sample := range samples {
			if sample.ok {
				okCount++
			}
		}
	}
	for _, t := range emptyTargets {
		delete(targets, t)
	}
	if len(targets) == 0 {
		delete(m.samples, serviceID)
		return m.emptyAvailability, nil
	}
	if total == 0 {
		return m.emptyAvailability, nil
	}
	return float64(okCount) / float64(total), nil
}
