package config

import "os"

type Config struct {
	PGDSN    string
	HTTPAddr string
}

func Load() Config {
	cfg := Config{
		PGDSN:    getenv("PG_DSN", "postgres://user:pass@localhost:5432/tranche?sslmode=disable"),
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
