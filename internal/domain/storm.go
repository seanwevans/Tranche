package domain

import "time"

type StormKind string

const (
	StormKindCloudflareDNSGlobal     StormKind = "CF_DNS_GLOBAL"
	StormKindCloudflareProxyDegraded           = "CF_PROXY_DEGRADED"
)

type StormPolicy struct {
	ID                int64
	CustomerID        int64
	ServiceID         int64
	Kind              StormKind
	ThresholdAvail    float64
	Window            time.Duration
	Cooldown          time.Duration
	MaxCoverageFactor float64
}
