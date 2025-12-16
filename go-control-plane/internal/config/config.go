package config

import (
	"os"
)

// Config holds runtime settings for the Go control plane.
type Config struct {
	HTTPAddr         string
	PostgresDSN      string
	QueueBackend     string
	QueueFile        string
	RedisURL         string
	RedisKey         string
	KafkaBrokers     string
	KafkaTopic       string
	WorkerWebhookURL string
	WorkerPlanURL    string
	WorkerToken      string
	WorkerLocalCmd   string
	SkipMigrate      bool
	InputDir         string
	SettingsPath     string
}

// FromEnv loads configuration with sensible defaults.
func FromEnv() Config {
	cfg := Config{
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		PostgresDSN:      getenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/refinery?sslmode=disable"),
		QueueBackend:     getenv("QUEUE_BACKEND", "file"),
		QueueFile:        getenv("QUEUE_FILE", "/tmp/refinery/retry_queue.json"),
		RedisURL:         getenv("REDIS_URL", ""),
		RedisKey:         getenv("REDIS_KEY", "refinery:queue"),
		KafkaBrokers:     getenv("KAFKA_BROKERS", ""),
		KafkaTopic:       getenv("KAFKA_TOPIC", "refinery.queue"),
		WorkerWebhookURL: getenv("WORKER_WEBHOOK_URL", ""),
		WorkerPlanURL:    getenv("WORKER_PLAN_URL", ""),
		WorkerToken:      getenv("WORKER_TOKEN", ""),
		WorkerLocalCmd:   getenv("WORKER_LOCAL_CMD", ""),
		SkipMigrate:      getenv("CP_SKIP_MIGRATE", "") != "",
		InputDir:         getenv("INPUT_DIR", "/input"),
		SettingsPath:     getenv("SETTINGS_PATH", "/config/settings.json"),
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
