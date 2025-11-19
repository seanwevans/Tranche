package dns

import "context"

type Logger interface {
	Printf(string, ...any)
}

type Provider interface {
	SetWeights(ctx context.Context, domain string, primaryWeight, backupWeight int) error
}

type NoopProvider struct {
	log Logger
}

func NewNoopProvider(log Logger) *NoopProvider {
	return &NoopProvider{log: log}
}

func (p *NoopProvider) SetWeights(_ context.Context, domain string, primaryWeight, backupWeight int) error {
	p.log.Printf("noop SetWeights(%s, primary=%d, backup=%d)", domain, primaryWeight, backupWeight)
	return nil
}
