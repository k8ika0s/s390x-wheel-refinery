package config

import (
	"os"
)

// Config holds runtime settings for the Go control plane.
type Config struct {
	HTTPAddr    string
	PostgresDSN string
	QueueFile   string
}

// FromEnv loads configuration with sensible defaults.
func FromEnv() Config {
	cfg := Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		PostgresDSN: getenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/refinery?sslmode=disable"),
		QueueFile:   getenv("QUEUE_FILE", "/tmp/refinery/retry_queue.json"),
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
