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
	AWSRegion    string
	AWSAccessKey string
	AWSSecretKey string
	AWSSession   string
}

func Load() Config {
	cfg := Config{
		PGDSN:        getenv("PG_DSN", "postgres://user:pass@localhost:5432/tranche?sslmode=disable"),
		HTTPAddr:     getenv("HTTP_ADDR", ":8080"),
		ProbePath:    getenv("PROBE_PATH", "/healthz"),
		ProbeTimeout: durationEnv("PROBE_TIMEOUT", 5*time.Second),
		AWSRegion:    os.Getenv("AWS_REGION"),
		AWSAccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSSession:   os.Getenv("AWS_SESSION_TOKEN"),
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
