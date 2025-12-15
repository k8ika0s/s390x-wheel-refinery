package service

import (
	"os"
	"strconv"
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/cas"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/objectstore"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
)

// Config holds worker settings.
type Config struct {
	HTTPAddr            string
	QueueBackend        string
	QueueFile           string
	RedisURL            string
	RedisKey            string
	KafkaBrokers        string
	KafkaTopic          string
	InputDir            string
	OutputDir           string
	CacheDir            string
	PythonVersion       string
	PlatformTag         string
	ContainerImage      string
	ContainerPreset     string
	WorkerToken         string
	ControlPlaneURL     string
	ControlPlaneToken   string
	PodmanBin           string
	RunnerTimeoutSec    int
	RequeueOnFailure    bool
	MaxRequeueAttempts  int
	AutorunInterval     int
	BatchSize           int
	RunCmd              []string
	IndexURL            string
	ExtraIndexURL       string
	UpgradeStrategy     string
	RequirementsPath    string
	ConstraintsPath     string
	CASRegistryURL      string
	CASRegistryRepo     string
	CASRegistryUser     string
	CASRegistryPass     string
	PackCatalog         *pack.Catalog
	ObjectStoreEndpoint string
	ObjectStoreBucket   string
	ObjectStoreAccess   string
	ObjectStoreSecret   string
	ObjectStoreUseSSL   bool
	LocalCASDir         string
	CASPushEnabled      bool
	RepairPushEnabled   bool
	RepairToolVersion   string
	RepairPolicyHash    string
	PackPushEnabled     bool
	RuntimePushEnabled  bool
}

func fromEnv() Config {
	cfg := Config{
		HTTPAddr:            getenv("WORKER_HTTP_ADDR", ":9000"),
		QueueBackend:        getenv("QUEUE_BACKEND", "file"),
		QueueFile:           getenv("QUEUE_FILE", "/tmp/refinery/retry_queue.json"),
		RedisURL:            getenv("REDIS_URL", ""),
		RedisKey:            getenv("REDIS_KEY", "refinery:queue"),
		KafkaBrokers:        getenv("KAFKA_BROKERS", ""),
		KafkaTopic:          getenv("KAFKA_TOPIC", "refinery.queue"),
		InputDir:            getenv("INPUT_DIR", "/input"),
		OutputDir:           getenv("OUTPUT_DIR", "/output"),
		CacheDir:            getenv("CACHE_DIR", "/cache"),
		PythonVersion:       getenv("PYTHON_VERSION", "3.11"),
		PlatformTag:         getenv("PLATFORM_TAG", "manylinux2014_s390x"),
		ContainerImage:      getenv("CONTAINER_IMAGE", ""),
		ContainerPreset:     getenv("CONTAINER_PRESET", "rocky"),
		WorkerToken:         getenv("WORKER_TOKEN", ""),
		ControlPlaneURL:     getenv("CONTROL_PLANE_URL", ""),
		ControlPlaneToken:   getenv("CONTROL_PLANE_TOKEN", ""),
		PodmanBin:           getenv("PODMAN_BIN", ""), // empty = stub podman; set to podman binary to execute
		RunnerTimeoutSec:    getenvInt("RUNNER_TIMEOUT_SEC", 900),
		RequeueOnFailure:    getenvBool("REQUEUE_ON_FAILURE", false),
		MaxRequeueAttempts:  getenvInt("MAX_REQUEUE_ATTEMPTS", 3),
		BatchSize:           getenvInt("BATCH_SIZE", 50),
		RunCmd:              parseCmd(getenv("WORKER_RUN_CMD", "")),
		IndexURL:            getenv("INDEX_URL", ""),
		ExtraIndexURL:       getenv("EXTRA_INDEX_URL", ""),
		UpgradeStrategy:     getenv("UPGRADE_STRATEGY", "pinned"),
		RequirementsPath:    getenv("REQUIREMENTS_PATH", ""),
		ConstraintsPath:     getenv("CONSTRAINTS_PATH", ""),
		CASRegistryURL:      getenv("CAS_REGISTRY_URL", ""),
		CASRegistryRepo:     getenv("CAS_REGISTRY_REPO", "artifacts"),
		CASRegistryUser:     getenv("CAS_REGISTRY_USER", ""),
		CASRegistryPass:     getenv("CAS_REGISTRY_PASSWORD", ""),
		ObjectStoreEndpoint: getenv("OBJECT_STORE_ENDPOINT", ""),
		ObjectStoreBucket:   getenv("OBJECT_STORE_BUCKET", ""),
		ObjectStoreAccess:   getenv("OBJECT_STORE_ACCESS_KEY", ""),
		ObjectStoreSecret:   getenv("OBJECT_STORE_SECRET_KEY", ""),
		ObjectStoreUseSSL:   getenvBool("OBJECT_STORE_USE_SSL", false),
		LocalCASDir:         getenv("LOCAL_CAS_DIR", "/cache/cas"),
		CASPushEnabled:      getenvBool("CAS_PUSH_ENABLED", false),
		RepairPushEnabled:   getenvBool("REPAIR_PUSH_ENABLED", false),
		RepairToolVersion:   getenv("REPAIR_TOOL_VERSION", ""),
		RepairPolicyHash:    getenv("REPAIR_POLICY_HASH", ""),
		PackPushEnabled:     getenvBool("PACK_PUSH_ENABLED", false),
		RuntimePushEnabled:  getenvBool("RUNTIME_PUSH_ENABLED", false),
	}
	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func getenvBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	}
	return def
}

func parseCmd(cmd string) []string {
	if cmd == "" {
		return nil
	}
	return strings.Fields(cmd)
}

// CASStore builds a CAS store client from config (Zot by default).
func (c Config) CASStore() cas.Store {
	if c.CASRegistryURL == "" {
		return cas.NullStore{}
	}
	repo := c.CASRegistryRepo
	if repo == "" {
		repo = "artifacts"
	}
	return cas.ZotStore{
		BaseURL:  c.CASRegistryURL,
		Repo:     repo,
		Username: c.CASRegistryUser,
		Password: c.CASRegistryPass,
	}
}

// CASFetcher returns a fetcher for pulling blobs locally.
func (c Config) CASFetcher() cas.Fetcher {
	repo := c.CASRegistryRepo
	if repo == "" {
		repo = "artifacts"
	}
	return cas.Fetcher{
		BaseURL:  c.CASRegistryURL,
		Repo:     repo,
		Username: c.CASRegistryUser,
		Password: c.CASRegistryPass,
	}
}

// ObjectStore builds an object storage client if configured.
func (c Config) ObjectStore() objectstore.Store {
	if c.ObjectStoreEndpoint == "" || c.ObjectStoreBucket == "" {
		return objectstore.NullStore{}
	}
	store, err := objectstore.NewMinIOStore(c.ObjectStoreEndpoint, c.ObjectStoreAccess, c.ObjectStoreSecret, c.ObjectStoreBucket, c.ObjectStoreUseSSL)
	if err != nil {
		return objectstore.NullStore{}
	}
	return store
}
