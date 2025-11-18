package config

import (
	"os"
	"time"
)

type Config struct {
	PGDSN        string
	HTTPAddr     string
	ProbePath    string
	ProbeTimeout time.Duration
}

func Load() Config {
	cfg := Config{
		PGDSN:        getenv("PG_DSN", "postgres://user:pass@localhost:5432/tranche?sslmode=disable"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		ProbePath:    getenv("PROBE_PATH", "/healthz"),
		ProbeTimeout: durationEnv("PROBE_TIMEOUT", 5*time.Second),
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
