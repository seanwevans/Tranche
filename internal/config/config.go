package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
        PGDSN                  string
        HTTPAddr               string
        MetricsAddr            string
        ProbePath              string
        ProbeTimeout           time.Duration
	BillingPeriod          time.Duration
	BillingRateCentsPerGB  int64
	BillingDiscountRate    float64
	UsageWindow            time.Duration
	UsageLookback          time.Duration
	UsageTick              time.Duration
	ControlPlaneAdminToken string
	AWSRegion              string
	AWSAccessKey           string
	AWSSecretKey           string
	AWSSession             string
	CDNDefaultProvider     string
	CDNServiceProviders    map[int64]string
	CDNCustomerProviders   map[int64]string
	CloudflareAccountID    string
	CloudflareAPIToken     string
	Cloudflare             CloudflareConfig
}

type CloudflareConfig struct {
	APIToken       string
	DefaultAccount string
	ZoneConfigJSON string
}

func Load() Config {
        cfg := Config{
                ControlPlaneAdminToken: os.Getenv("CONTROL_PLANE_ADMIN_TOKEN"),
                CloudflareAccountID:    os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
                CloudflareAPIToken:     os.Getenv("CLOUDFLARE_API_TOKEN"),
                AWSRegion:              os.Getenv("AWS_REGION"),
                AWSAccessKey:           os.Getenv("AWS_ACCESS_KEY_ID"),
                AWSSecretKey:           os.Getenv("AWS_SECRET_ACCESS_KEY"),
                AWSSession:             os.Getenv("AWS_SESSION_TOKEN"),
                PGDSN:                  getenv("PG_DSN", "postgres://user:pass@localhost:5432/tranche?sslmode=disable"),
                HTTPAddr:               getenv("HTTP_ADDR", ":8080"),
                MetricsAddr:            getenv("METRICS_ADDR", ":9090"),
                ProbePath:              getenv("PROBE_PATH", "/healthz"),
                ProbeTimeout:           durationEnv("PROBE_TIMEOUT", 5*time.Second),
		BillingPeriod:          durationEnv("BILLING_PERIOD", 24*time.Hour),
		BillingRateCentsPerGB:  intEnv("BILLING_RATE_CENTS_PER_GB", 12),
		BillingDiscountRate:    floatEnv("BILLING_DISCOUNT_RATE", 0.5),
		CDNDefaultProvider:     getenv("CDN_DEFAULT_PROVIDER", ""),
		CDNServiceProviders:    parseProviderOverrides("CDN_PROVIDER_SERVICE_OVERRIDES"),
		CDNCustomerProviders:   parseProviderOverrides("CDN_PROVIDER_CUSTOMER_OVERRIDES"),
		Cloudflare: CloudflareConfig{
			APIToken:       os.Getenv("CLOUDFLARE_API_TOKEN"),
			DefaultAccount: getenv("CLOUDFLARE_ACCOUNT_ID", ""),
			ZoneConfigJSON: os.Getenv("CLOUDFLARE_ZONE_CONFIG"),
		},
		UsageWindow:   durationEnv("USAGE_WINDOW", time.Hour),
		UsageLookback: durationEnv("USAGE_LOOKBACK", 6*time.Hour),
		UsageTick:     durationEnv("USAGE_TICK", 5*time.Minute),
	}

	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func intEnv(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if iv, err := strconv.ParseInt(v, 10, 64); err == nil {
			return iv
		}
	}
	return def
}

func floatEnv(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if fv, err := strconv.ParseFloat(v, 64); err == nil {
			return fv
		}
	}
	return def
}

func parseProviderOverrides(envKey string) map[int64]string {
	val := os.Getenv(envKey)
	if val == "" {
		return map[int64]string{}
	}

	table := make(map[int64]string)
	entries := strings.Split(val, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		table[id] = parts[1]
	}
	return table
}
