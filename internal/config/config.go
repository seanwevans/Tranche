package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	PGDSN                 string
	HTTPAddr              string
	ProbePath             string
	ProbeTimeout          time.Duration
	BillingPeriod         time.Duration
	BillingRateCentsPerGB int64
	BillingDiscountRate   float64
}

func Load() Config {
	cfg := Config{
		PGDSN:                 getenv("PG_DSN", "postgres://user:pass@localhost:5432/tranche?sslmode=disable"),
		HTTPAddr:              getenv("HTTP_ADDR", ":8080"),
		ProbePath:             getenv("PROBE_PATH", "/healthz"),
		ProbeTimeout:          durationEnv("PROBE_TIMEOUT", 5*time.Second),
		BillingPeriod:         durationEnv("BILLING_PERIOD", 24*time.Hour),
		BillingRateCentsPerGB: intEnv("BILLING_RATE_CENTS_PER_GB", 12),
		BillingDiscountRate:   floatEnv("BILLING_DISCOUNT_RATE", 0.5),
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
