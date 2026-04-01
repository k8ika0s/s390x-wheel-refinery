package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Build duration histogram with buckets optimized for wheel builds
	BuildDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_build_duration_seconds",
			Help: "Duration of build operations in seconds",
			Buckets: []float64{
				10, 30, 60, 120, 300, 600, 900, 1200, 1800, 3600, 7200,
			}, // 10s to 2h
		},
		[]string{"package", "status"},
	)

	// Build phase duration for detailed performance tracking
	BuildPhaseDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_build_phase_duration_seconds",
			Help: "Duration of individual build phases in seconds",
			Buckets: []float64{
				5, 15, 30, 60, 120, 300, 600, 900, 1200, 1800,
			},
		},
		[]string{"phase", "package"},
	)

	// Queue wait time histogram
	QueueWaitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_queue_wait_duration_seconds",
			Help: "Time items spend waiting in queue before processing",
			Buckets: []float64{
				1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600,
			}, // 1s to 1h
		},
		[]string{"queue"},
	)

	// Queue depth gauge
	QueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_queue_depth",
			Help: "Current number of items in queue",
		},
		[]string{"queue"},
	)

	// Build counters
	BuildsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_builds_total",
			Help: "Total number of builds",
		},
		[]string{"package"},
	)

	BuildsSuccess = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_builds_success_total",
			Help: "Total number of successful builds",
		},
		[]string{"package"},
	)

	BuildsFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_builds_failed_total",
			Help: "Total number of failed builds",
		},
		[]string{"package", "reason"},
	)

	// Build status distribution
	BuildStatusCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_build_status_count",
			Help: "Number of builds in each status",
		},
		[]string{"status"},
	)

	// Build retry counter
	BuildRetries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_build_retries_total",
			Help: "Total number of build retries",
		},
		[]string{"package"},
	)

	// Hint metrics
	HintApplied = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_hint_applied_total",
			Help: "Total number of hints applied",
		},
		[]string{"hint_type", "package"},
	)

	HintSuccess = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_hint_success_total",
			Help: "Total number of successful hint applications",
		},
		[]string{"hint_type", "package"},
	)

	// CAS metrics
	CASHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "refinery_cas_hits",
			Help: "Total number of CAS cache hits",
		},
	)

	CASMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "refinery_cas_misses",
			Help: "Total number of CAS cache misses",
		},
	)

	CASOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_cas_operation_duration_seconds",
			Help: "Duration of CAS operations in seconds",
			Buckets: []float64{
				0.1, 0.5, 1, 2, 5, 10, 30, 60,
			},
		},
		[]string{"operation"}, // push, fetch, check
	)

	CASBytesTransferred = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_cas_bytes_transferred_total",
			Help: "Total bytes transferred to/from CAS",
		},
		[]string{"operation"}, // push, fetch
	)

	CASArtifactsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_cas_artifacts_total",
			Help: "Total number of artifacts in CAS",
		},
	)

	CASArtifactsByType = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_cas_artifacts_by_type",
			Help: "Number of artifacts by type in CAS",
		},
		[]string{"type"}, // runtime, pack, wheel, repair
	)

	CASStorageUsed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_cas_storage_used_bytes",
			Help: "Storage space used by CAS in bytes",
		},
	)

	CASStorageTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_cas_storage_total_bytes",
			Help: "Total storage space available for CAS in bytes",
		},
	)

	CASErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_cas_errors_total",
			Help: "Total number of CAS errors",
		},
		[]string{"error_type"},
	)

	// MinIO/Object Storage metrics
	MinIOStorageUsed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_minio_storage_used_bytes",
			Help: "Storage space used by MinIO in bytes",
		},
	)

	MinIOStorageTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_minio_storage_total_bytes",
			Help: "Total storage space available for MinIO in bytes",
		},
	)

	MinIOErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_minio_errors_total",
			Help: "Total number of MinIO errors",
		},
		[]string{"error_type"},
	)

	// Worker metrics
	WorkersOnline = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_workers_online",
			Help: "Number of workers currently online",
		},
	)

	WorkerOnline = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_online",
			Help: "Worker online status (1=online, 0=offline)",
		},
		[]string{"worker_id"},
	)

	WorkerActiveBuilds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_active_builds",
			Help: "Number of active builds on worker",
		},
		[]string{"worker_id"},
	)

	WorkerMaxConcurrentBuilds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_max_concurrent_builds",
			Help: "Maximum concurrent builds allowed on worker",
		},
		[]string{"worker_id"},
	)

	WorkerCPUUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_cpu_usage_seconds",
			Help: "Worker CPU usage in seconds",
		},
		[]string{"worker_id"},
	)

	WorkerCPULimit = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_cpu_limit_seconds",
			Help: "Worker CPU limit in seconds",
		},
		[]string{"worker_id"},
	)

	WorkerMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_memory_usage_bytes",
			Help: "Worker memory usage in bytes",
		},
		[]string{"worker_id"},
	)

	WorkerMemoryLimit = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_memory_limit_bytes",
			Help: "Worker memory limit in bytes",
		},
		[]string{"worker_id"},
	)

	WorkerPollDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "refinery_worker_poll_duration_seconds",
			Help:    "Duration of worker poll operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"worker_id"},
	)

	WorkerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_errors_total",
			Help: "Total number of worker errors",
		},
		[]string{"worker_id", "error_type"},
	)

	WorkerLastHeartbeat = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "refinery_worker_last_heartbeat_timestamp",
			Help: "Timestamp of worker's last heartbeat",
		},
		[]string{"worker_id"},
	)
)

// Made with Bob
