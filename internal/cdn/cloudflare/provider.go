package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cflog "github.com/cloudflare/cloudflare-go"

	"tranche/internal/cdn"
	"tranche/internal/config"
	"tranche/internal/db"
)

const providerName = "cloudflare"

type ZoneConfig struct {
	ZoneID    string `json:"zone_id"`
	AccountID string `json:"account_id"`
}

type Provider struct {
	api    *cflog.API
	zones  map[string]ZoneConfig
	logger cdn.Logger
}

var _ cdn.UsageProvider = (*Provider)(nil)

func NewProvider(cfg config.CloudflareConfig, logger cdn.Logger) (*Provider, error) {
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("cloudflare api token missing")
	}

	api, err := cflog.NewWithAPIToken(cfg.APIToken)
	if err != nil {
		return nil, fmt.Errorf("init cloudflare client: %w", err)
	}

	zoneMap := make(map[string]ZoneConfig)
	if cfg.ZoneConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ZoneConfigJSON), &zoneMap); err != nil {
			return nil, fmt.Errorf("parse CLOUDFLARE_ZONE_CONFIG: %w", err)
		}
	}

	for key, zone := range zoneMap {
		if zone.AccountID == "" {
			zone.AccountID = cfg.DefaultAccount
			zoneMap[key] = zone
		}
	}

	return &Provider{api: api, zones: zoneMap, logger: logger}, nil
}

func (p *Provider) Name() string {
	return providerName
}

func (p *Provider) FetchUsage(ctx context.Context, svc db.Service, since, until time.Time) (int64, int64, error) {
	primaryZone, err := p.zoneForAlias(svc.PrimaryCdn)
	if err != nil {
		return 0, 0, err
	}
	backupZone, err := p.zoneForAlias(svc.BackupCdn)
	if err != nil {
		return 0, 0, err
	}

	primaryBytes, err := p.zoneBytes(ctx, primaryZone.ZoneID, since, until)
	if err != nil {
		return 0, 0, err
	}
	backupBytes, err := p.zoneBytes(ctx, backupZone.ZoneID, since, until)
	if err != nil {
		return 0, 0, err
	}

	return primaryBytes, backupBytes, nil
}

func (p *Provider) zoneBytes(ctx context.Context, zoneID string, since, until time.Time) (int64, error) {
	continuous := true
	resp, err := p.api.ZoneAnalyticsDashboard(ctx, zoneID, cflog.ZoneAnalyticsOptions{Since: &since, Until: &until, Continuous: &continuous})
	if err != nil {
		return 0, fmt.Errorf("cloudflare analytics for zone %s: %w", zoneID, err)
	}
	return int64(resp.Totals.Bandwidth.All), nil
}

func (p *Provider) zoneForAlias(alias string) (ZoneConfig, error) {
	zone, ok := p.zones[alias]
	if !ok || zone.ZoneID == "" {
		return ZoneConfig{}, fmt.Errorf("zone mapping for %q not found", alias)
	}
	return zone, nil
}
