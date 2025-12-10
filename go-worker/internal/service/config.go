package service

import "os"

// Config holds worker settings.
type Config struct {
	HTTPAddr      string
	QueueBackend  string
	QueueFile     string
	RedisURL      string
	RedisKey      string
	KafkaBrokers  string
	KafkaTopic    string
	InputDir      string
	OutputDir     string
	CacheDir      string
	PythonVersion string
	PlatformTag   string
	ContainerImage  string
	ContainerPreset string
	WorkerToken     string
	ControlPlaneURL string
	ControlPlaneToken string
	PodmanBin       string
	AutorunInterval int
}

func fromEnv() Config {
	cfg := Config{
		HTTPAddr:        getenv("WORKER_HTTP_ADDR", ":9000"),
		QueueBackend:    getenv("QUEUE_BACKEND", "file"),
		QueueFile:       getenv("QUEUE_FILE", "/tmp/refinery/retry_queue.json"),
		RedisURL:        getenv("REDIS_URL", ""),
		RedisKey:        getenv("REDIS_KEY", "refinery:queue"),
		KafkaBrokers:    getenv("KAFKA_BROKERS", ""),
		KafkaTopic:      getenv("KAFKA_TOPIC", "refinery.queue"),
		InputDir:        getenv("INPUT_DIR", "/input"),
		OutputDir:       getenv("OUTPUT_DIR", "/output"),
		CacheDir:        getenv("CACHE_DIR", "/cache"),
		PythonVersion:   getenv("PYTHON_VERSION", "3.11"),
		PlatformTag:     getenv("PLATFORM_TAG", "manylinux2014_s390x"),
		ContainerImage:  getenv("CONTAINER_IMAGE", ""),
		ContainerPreset: getenv("CONTAINER_PRESET", "rocky"),
		WorkerToken:     getenv("WORKER_TOKEN", ""),
		ControlPlaneURL: getenv("CONTROL_PLANE_URL", ""),
		ControlPlaneToken: getenv("CONTROL_PLANE_TOKEN", ""),
		PodmanBin:       getenv("PODMAN_BIN", ""), // empty = stub podman; set to podman binary to execute
	}
	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
