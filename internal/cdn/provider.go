package cdn

import (
	"context"
	"fmt"
	"time"

	"tranche/internal/db"
)

// UsageProvider exposes CDN-specific primitives such as usage gathering.
// Implementations should be stateless and safe for concurrent use.
type UsageProvider interface {
	Name() string
	FetchUsage(ctx context.Context, svc db.Service, since, until time.Time) (primaryBytes, backupBytes int64, err error)
}

type Logger interface {
	Printf(string, ...any)
}

type SelectorConfig struct {
	DefaultProvider   string
	CustomerOverrides map[int64]string
	ServiceOverrides  map[int64]string
	Providers         []UsageProvider
}

type Selector struct {
	providers         map[string]UsageProvider
	defaultProvider   string
	customerOverrides map[int64]string
	serviceOverrides  map[int64]string
}

func NewSelector(cfg SelectorConfig) (*Selector, error) {
	providers := make(map[string]UsageProvider)
	for _, p := range cfg.Providers {
		if p == nil {
			continue
		}
		name := p.Name()
		if name == "" {
			return nil, fmt.Errorf("provider with empty name")
		}
		providers[name] = p
	}

	return &Selector{
		providers:         providers,
		defaultProvider:   cfg.DefaultProvider,
		customerOverrides: cfg.CustomerOverrides,
		serviceOverrides:  cfg.ServiceOverrides,
	}, nil
}

func (s *Selector) ProviderForService(svc db.Service) (UsageProvider, error) {
	name := svc.PrimaryCdn
	if override, ok := s.customerOverrides[svc.CustomerID]; ok {
		name = override
	}
	if override, ok := s.serviceOverrides[svc.ID]; ok {
		name = override
	}
	if name == "" {
		name = s.defaultProvider
	}
	if name == "" {
		return nil, fmt.Errorf("no provider configured for service %d", svc.ID)
	}
	p, ok := s.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}
