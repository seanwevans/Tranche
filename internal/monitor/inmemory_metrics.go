package monitor

import (
	"sync"
	"time"
)

type probeSample struct {
	t  time.Time
	ok bool
}

type InMemoryMetrics struct {
	mu      sync.Mutex
	samples map[int64][]probeSample
}

func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		samples: make(map[int64][]probeSample),
	}
}

func (m *InMemoryMetrics) RecordProbe(serviceID int64, ok bool, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples[serviceID] = append(m.samples[serviceID], probeSample{
		t:  time.Now(),
		ok: ok,
	})
}

func (m *InMemoryMetrics) Availability(serviceID int64, window time.Duration) float64 {
	cutoff := time.Now().Add(-window)
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.samples[serviceID]
	if len(s) == 0 {
		return 1.0
	}

	// drop old
	idx := len(s)
	for i, sample := range s {
		if sample.t.After(cutoff) {
			idx = i
			break
		}
	}
	s = s[idx:]
	if len(s) == 0 {
		m.samples[serviceID] = nil
		return 1.0
	}
	m.samples[serviceID] = s

	total := len(s)
	okCount := 0
	for _, sample := range s {
		if sample.ok {
			okCount++
		}
	}
	return float64(okCount) / float64(total)
}
