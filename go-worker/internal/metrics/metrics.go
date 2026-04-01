package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Worker status metrics
	WorkerOnline = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_online",
			Help: "Worker online status (1=online, 0=offline)",
		},
	)

	WorkerActiveBuilds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_active_builds",
			Help: "Number of active builds on this worker",
		},
	)

	WorkerMaxConcurrentBuilds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_max_concurrent_builds",
			Help: "Maximum concurrent builds allowed on this worker",
		},
	)

	// Build execution metrics
	BuildsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_builds_processed_total",
			Help: "Total number of builds processed by this worker",
		},
		[]string{"status"}, // success, failed
	)

	BuildDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_worker_build_duration_seconds",
			Help: "Duration of build operations in seconds",
			Buckets: []float64{
				10, 30, 60, 120, 300, 600, 900, 1200, 1800, 3600, 7200,
			},
		},
		[]string{"package", "status"},
	)

	BuildPhaseDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_worker_build_phase_duration_seconds",
			Help: "Duration of individual build phases in seconds",
			Buckets: []float64{
				5, 15, 30, 60, 120, 300, 600, 900, 1200, 1800,
			},
		},
		[]string{"phase"}, // runtime, pack, wheel, repair
	)

	// Queue polling metrics
	PollDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "refinery_worker_poll_duration_seconds",
			Help:    "Duration of queue poll operations",
			Buckets: prometheus.DefBuckets,
		},
	)

	PollErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_poll_errors_total",
			Help: "Total number of queue poll errors",
		},
		[]string{"error_type"},
	)

	// CAS operation metrics
	CASOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_worker_cas_operation_duration_seconds",
			Help: "Duration of CAS operations in seconds",
			Buckets: []float64{
				0.1, 0.5, 1, 2, 5, 10, 30, 60,
			},
		},
		[]string{"operation"}, // push, fetch, check
	)

	CASOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_cas_operations_total",
			Help: "Total number of CAS operations",
		},
		[]string{"operation", "status"}, // push/fetch/check, success/failed
	)

	CASBytesTransferred = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_cas_bytes_transferred_total",
			Help: "Total bytes transferred to/from CAS",
		},
		[]string{"operation"}, // push, fetch
	)

	// Hint application metrics
	HintsApplied = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_hints_applied_total",
			Help: "Total number of hints applied during builds",
		},
		[]string{"hint_type"},
	)

	HintsSuccessful = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_hints_successful_total",
			Help: "Total number of successful hint applications",
		},
		[]string{"hint_type"},
	)

	// Resource usage metrics
	CPUUsageSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_cpu_usage_seconds",
			Help: "Worker CPU usage in seconds",
		},
	)

	MemoryUsageBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_memory_usage_bytes",
			Help: "Worker memory usage in bytes",
		},
	)

	DiskUsageBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_disk_usage_bytes",
			Help: "Worker disk usage in bytes",
		},
	)

	// Cache metrics
	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "refinery_worker_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "refinery_worker_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	CacheSizeBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_cache_size_bytes",
			Help: "Current cache size in bytes",
		},
	)

	CacheEvictions = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "refinery_worker_cache_evictions_total",
			Help: "Total number of cache evictions",
		},
	)

	// Error tracking
	BuildErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_build_errors_total",
			Help: "Total number of build errors",
		},
		[]string{"error_type"},
	)

	WorkerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_errors_total",
			Help: "Total number of worker errors",
		},
		[]string{"error_type"},
	)

	// Heartbeat
	LastHeartbeat = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "refinery_worker_last_heartbeat_timestamp",
			Help: "Timestamp of worker's last heartbeat",
		},
	)

	// Container operations
	ContainerOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "refinery_worker_container_operations_total",
			Help: "Total number of container operations",
		},
		[]string{"operation", "status"}, // create/start/stop/remove, success/failed
	)

	ContainerOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "refinery_worker_container_operation_duration_seconds",
			Help: "Duration of container operations in seconds",
			Buckets: []float64{
				0.1, 0.5, 1, 2, 5, 10, 30, 60,
			},
		},
		[]string{"operation"}, // create, start, stop, remove
	)
)

// Made with Bob
