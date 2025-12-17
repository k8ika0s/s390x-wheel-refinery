package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds runtime settings for the Go control plane.
type Config struct {
	HTTPAddr            string
	PostgresDSN         string
	QueueBackend        string
	QueueFile           string
	RedisURL            string
	RedisKey            string
	PlanRedisKey        string
	KafkaBrokers        string
	KafkaTopic          string
	WorkerWebhookURL    string
	WorkerPlanURL       string
	WorkerToken         string
	WorkerLocalCmd      string
	SkipMigrate         bool
	SettingsPath        string
	AutoPlan            bool
	AutoBuild           bool
	HintsDir            string
	SeedHints           bool
	ObjectStoreEndpoint string
	ObjectStoreBucket   string
	ObjectStoreAccess   string
	ObjectStoreSecret   string
	ObjectStoreUseSSL   bool
	InputObjectPrefix   string
	CORSOrigins         []string
	CORSHeaders         []string
	CORSMethods         []string
	CORSCredentials     bool
	CORSMaxAge          int
}

// FromEnv loads configuration with sensible defaults.
func FromEnv() Config {
	cfg := Config{
		HTTPAddr:            getenv("HTTP_ADDR", ":8080"),
		PostgresDSN:         getenv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/refinery?sslmode=disable"),
		QueueBackend:        getenv("QUEUE_BACKEND", "file"),
		QueueFile:           getenv("QUEUE_FILE", "/tmp/refinery/retry_queue.json"),
		RedisURL:            getenv("REDIS_URL", ""),
		RedisKey:            getenv("REDIS_KEY", "refinery:queue"),
		PlanRedisKey:        getenv("PLAN_REDIS_KEY", "refinery:plan_queue"),
		KafkaBrokers:        getenv("KAFKA_BROKERS", ""),
		KafkaTopic:          getenv("KAFKA_TOPIC", "refinery.queue"),
		WorkerWebhookURL:    getenv("WORKER_WEBHOOK_URL", ""),
		WorkerPlanURL:       getenv("WORKER_PLAN_URL", ""),
		WorkerToken:         getenv("WORKER_TOKEN", ""),
		WorkerLocalCmd:      getenv("WORKER_LOCAL_CMD", ""),
		SkipMigrate:         getenv("CP_SKIP_MIGRATE", "") != "",
		SettingsPath:        getenv("SETTINGS_PATH", "/config/settings.json"),
		AutoPlan:            getenv("AUTO_PLAN", "0") != "0",
		AutoBuild:           getenv("AUTO_BUILD", "0") != "0",
		HintsDir:            getenv("HINTS_DIR", "/hints"),
		SeedHints:           getenv("HINTS_SEED", "1") != "0",
		ObjectStoreEndpoint: getenv("OBJECT_STORE_ENDPOINT", ""),
		ObjectStoreBucket:   getenv("OBJECT_STORE_BUCKET", ""),
		ObjectStoreAccess:   getenv("OBJECT_STORE_ACCESS_KEY", ""),
		ObjectStoreSecret:   getenv("OBJECT_STORE_SECRET_KEY", ""),
		ObjectStoreUseSSL:   getenvBool("OBJECT_STORE_USE_SSL", false),
		InputObjectPrefix:   getenv("INPUT_OBJECT_PREFIX", "inputs"),
		CORSOrigins:         parseCSV(getenv("CORS_ORIGINS", "")),
		CORSHeaders:         parseCSV(getenv("CORS_HEADERS", "Content-Type,Authorization,X-Worker-Token")),
		CORSMethods:         parseCSV(getenv("CORS_METHODS", "GET,POST,PUT,DELETE,OPTIONS")),
		CORSCredentials:     getenvBool("CORS_CREDENTIALS", false),
		CORSMaxAge:          getenvInt("CORS_MAX_AGE", 600),
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch v {
		case "1", "true", "TRUE", "yes", "YES", "on", "ON":
			return true
		case "0", "false", "FALSE", "no", "NO", "off", "OFF":
			return false
		default:
			return def
		}
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func parseCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
