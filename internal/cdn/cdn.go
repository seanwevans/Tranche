package cdn

import (
	"context"
	"time"
)

// WindowedUsage captures usage for a hostname within a discrete billing window.
type WindowedUsage struct {
	Host        string
	WindowStart time.Time
	WindowEnd   time.Time
	Bytes       int64
}

// Provider fetches usage statistics between aligned windows.
type Provider interface {
	Usage(ctx context.Context, start, end time.Time, window time.Duration, hosts []string) ([]WindowedUsage, error)
}
