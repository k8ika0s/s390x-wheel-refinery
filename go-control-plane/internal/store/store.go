package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/settings"
)

// ErrNotFound is returned when a requested record is missing.
var ErrNotFound = errors.New("not found")

// Event represents a build event history row.
type Event struct {
	RunID          string         `json:"run_id,omitempty"`
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	PythonTag      string         `json:"python_tag,omitempty"`
	PlatformTag    string         `json:"platform_tag,omitempty"`
	Status         string         `json:"status"`
	Detail         string         `json:"detail,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Timestamp      int64          `json:"timestamp"`
	MatchedHintIDs []string       `json:"matched_hint_ids,omitempty"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
}

// Hint represents a hint catalog entry.
type Hint struct {
	ID         string              `json:"id" yaml:"id"`
	Pattern    string              `json:"pattern" yaml:"pattern"`
	Recipes    map[string][]string `json:"recipes,omitempty" yaml:"recipes,omitempty"`
	Note       string              `json:"note,omitempty" yaml:"note,omitempty"`
	Tags       []string            `json:"tags,omitempty" yaml:"tags,omitempty"`
	Severity   string              `json:"severity,omitempty" yaml:"severity,omitempty"`
	AppliesTo  map[string][]string `json:"applies_to,omitempty" yaml:"applies_to,omitempty"`
	Confidence string              `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	Examples   []string            `json:"examples,omitempty" yaml:"examples,omitempty"`
	DeletedAt  *time.Time          `json:"deleted_at,omitempty" yaml:"deleted_at,omitempty"`
}

// LogEntry represents stored log metadata/content.
type LogEntry struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Content   string `json:"content,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// LogChunk represents a streamed log segment.
type LogChunk struct {
	ID        int64  `json:"id,omitempty"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	RunID     string `json:"run_id,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
	Seq       int64  `json:"seq,omitempty"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// PendingInput represents an uploaded requirements file awaiting planning.
type PendingInput struct {
	ID           int64           `json:"id"`
	Filename     string          `json:"filename"`
	Digest       string          `json:"digest,omitempty"`
	SizeBytes    int64           `json:"size_bytes,omitempty"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	SourceType   string          `json:"source_type,omitempty"`
	ObjectBucket string          `json:"object_bucket,omitempty"`
	ObjectKey    string          `json:"object_key,omitempty"`
	ContentType  string          `json:"content_type,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	PlanID       *int64          `json:"plan_id,omitempty"`
	LoadedAt     *time.Time      `json:"loaded_at,omitempty"`
	PlannedAt    *time.Time      `json:"planned_at,omitempty"`
	ProcessedAt  *time.Time      `json:"processed_at,omitempty"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ManifestEntry tracks output wheel metadata.
type ManifestEntry struct {
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Wheel          string          `json:"wheel"`
	WheelURL       string          `json:"wheel_url,omitempty"`
	RepairURL      string          `json:"repair_url,omitempty"`
	RepairDigest   string          `json:"repair_digest,omitempty"`
	RuntimeURL     string          `json:"runtime_url,omitempty"`
	PackURLs       []string        `json:"pack_urls,omitempty"`
	PythonTag      string          `json:"python_tag,omitempty"`
	PlatformTag    string          `json:"platform_tag,omitempty"`
	Status         string          `json:"status,omitempty"`
	ManifestDigest string          `json:"manifest_digest,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      int64           `json:"created_at,omitempty"`
}

// Artifact represents a downloadable/browsable build artifact.
type Artifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	URL     string `json:"url"`
}

// PlanHint captures a matched hint attached to a plan node.
type PlanHint struct {
	ID      string              `json:"id"`
	Pattern string              `json:"pattern,omitempty"`
	Note    string              `json:"note,omitempty"`
	Reason  string              `json:"reason,omitempty"`
	Tags    []string            `json:"tags,omitempty"`
	Recipes map[string][]string `json:"recipes,omitempty"`
}

// PlanRecipe captures a recipe or pack attached to a plan node.
type PlanRecipe struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// PlanNode describes a unit in the build plan/graph.
type PlanNode struct {
	NodeID        string       `json:"node_id,omitempty"`
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	PythonVersion string       `json:"python_version,omitempty"`
	PythonTag     string       `json:"python_tag,omitempty"`
	PlatformTag   string       `json:"platform_tag,omitempty"`
	Action        string       `json:"action"`
	Hints         []PlanHint   `json:"hints,omitempty"`
	Recipes       []PlanRecipe `json:"recipes,omitempty"`
}

// PlanSnapshot captures a stored plan with optional DAG payload.
type PlanSnapshot struct {
	ID     int64           `json:"id"`
	RunID  string          `json:"run_id,omitempty"`
	Plan   []PlanNode      `json:"plan"`
	DAG    json.RawMessage `json:"dag,omitempty"`
	Queued bool            `json:"queued,omitempty"`
}

// PlanSummary provides a compact plan list entry.
type PlanSummary struct {
	ID         int64  `json:"id"`
	RunID      string `json:"run_id,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	NodeCount  int    `json:"node_count"`
	BuildCount int    `json:"build_count"`
	Queued     bool   `json:"queued,omitempty"`
}

// BuildStatus tracks a build job derived from a plan.
type BuildStatus struct {
	ID             int64          `json:"id"`
	NodeID         string         `json:"node_id,omitempty"`
	WorkerID       string         `json:"worker_id,omitempty"`
	Package        string         `json:"package"`
	Version        string         `json:"version"`
	PythonTag      string         `json:"python_tag"`
	PlatformTag    string         `json:"platform_tag"`
	Status         string         `json:"status"`
	PreviousStatus string         `json:"previous_status,omitempty"`
	Attempts       int            `json:"attempts"`
	LastError      string         `json:"last_error,omitempty"`
	FailureSummary string         `json:"failure_summary,omitempty"`
	OldestAgeSec   int64          `json:"oldest_age_seconds,omitempty"`
	StaleAgeSec    int64          `json:"stale_age_seconds,omitempty"`
	CreatedAt      int64          `json:"created_at"`
	UpdatedAt      int64          `json:"updated_at"`
	LeasedAt       int64          `json:"leased_at,omitempty"`
	StartedAt      int64          `json:"started_at,omitempty"`
	FinishedAt     int64          `json:"finished_at,omitempty"`
	RunID          string         `json:"run_id,omitempty"`
	PlanID         int64          `json:"plan_id,omitempty"`
	BackoffUntil   int64          `json:"backoff_until,omitempty"`
	BackoffReason  string         `json:"backoff_reason,omitempty"`
	BackoffSeconds int            `json:"backoff_seconds,omitempty"`
	ReasonCode     string         `json:"reason_code,omitempty"`
	ReasonDetail   string         `json:"reason_detail,omitempty"`
	Recipes        []string       `json:"recipes,omitempty"`
	HintIDs        []string       `json:"hint_ids,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// BuildAttempt captures per-attempt build history.
type BuildAttempt struct {
	ID             int64          `json:"id"`
	NodeID         string         `json:"node_id,omitempty"`
	Package        string         `json:"package"`
	Version        string         `json:"version"`
	Attempt        int            `json:"attempt"`
	Status         string         `json:"status"`
	LastError      string         `json:"last_error,omitempty"`
	FailureSummary string         `json:"failure_summary,omitempty"`
	BackoffUntil   int64          `json:"backoff_until,omitempty"`
	BackoffReason  string         `json:"backoff_reason,omitempty"`
	BackoffSeconds int            `json:"backoff_seconds,omitempty"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
	Recipes        []string       `json:"recipes,omitempty"`
	HintIDs        []string       `json:"hint_ids,omitempty"`
	ReasonCode     string         `json:"reason_code,omitempty"`
	ReasonDetail   string         `json:"reason_detail,omitempty"`
	StartedAt      int64          `json:"started_at,omitempty"`
	FinishedAt     int64          `json:"finished_at,omitempty"`
	RunID          string         `json:"run_id,omitempty"`
	PlanID         int64          `json:"plan_id,omitempty"`
	CreatedAt      int64          `json:"created_at,omitempty"`
	UpdatedAt      int64          `json:"updated_at,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// BuildQueueStats captures aggregate queue counts.
type BuildQueueStats struct {
	Length       int   `json:"length"`
	OldestAgeSec int64 `json:"oldest_age_seconds,omitempty"`
	Pending      int   `json:"pending"`
	Retry        int   `json:"retry"`
	Leased       int   `json:"leased"`
	Building     int   `json:"building"`
}

// WorkerStatus tracks worker heartbeat metadata.
type WorkerStatus struct {
	WorkerID             string         `json:"worker_id"`
	RunID                string         `json:"run_id,omitempty"`
	LastSeen             int64          `json:"last_seen"`
	ActiveBuilds         int            `json:"active_builds"`
	BuildPoolSize        int            `json:"build_pool_size"`
	PlanPoolSize         int            `json:"plan_pool_size"`
	HeartbeatIntervalSec int            `json:"heartbeat_interval_sec,omitempty"`
	CASHits              int64          `json:"cas_hits,omitempty"`
	CASMisses            int64          `json:"cas_misses,omitempty"`
	CreatedAt            int64          `json:"created_at,omitempty"`
	UpdatedAt            int64          `json:"updated_at,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

// PackageSummary aggregates status for a package.
type PackageSummary struct {
	Name         string         `json:"name"`
	StatusCounts map[string]int `json:"status_counts"`
	Latest       *Event         `json:"latest,omitempty"`
}

// Summary aggregates recent status counts and failures.
type Summary struct {
	StatusCounts map[string]int `json:"status_counts"`
	Failures     []Event        `json:"failures"`
}

// Stat is a simple key/value for leaderboard style metrics.
type Stat struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// BuildAttemptStats summarizes attempts in a recent window.
type BuildAttemptStats struct {
	Total                 int     `json:"total"`
	Built                 int     `json:"built"`
	Failed                int     `json:"failed"`
	Retry                 int     `json:"retry"`
	Quarantined           int     `json:"quarantined"`
	AvgDurationMs         float64 `json:"avg_duration_ms"`
	FirstAttemptBuilt     int     `json:"first_attempt_built,omitempty"`
	FirstAttemptFailed    int     `json:"first_attempt_failed,omitempty"`
	RetryAttemptBuilt     int     `json:"retry_attempt_built,omitempty"`
	RetryAttemptFailed    int     `json:"retry_attempt_failed,omitempty"`
	HintApplied           int     `json:"hint_applied,omitempty"`
	KnownHintApplied      int     `json:"known_hint_applied,omitempty"`
	HeuristicApplied      int     `json:"heuristic_applied,omitempty"`
	LLMApplied            int     `json:"llm_applied,omitempty"`
	LLMSuggestionsIgnored int     `json:"llm_suggestions_ignored,omitempty"`
	HintSaveFailed        int     `json:"hint_save_failed,omitempty"`
	StaleRequeues         int     `json:"stale_requeues,omitempty"`
	PackageUnavailable    int     `json:"package_unavailable,omitempty"`
	PackFallbackAttempts  int     `json:"pack_fallback_attempts,omitempty"`
	PackFallbackSuccesses int     `json:"pack_fallback_successes,omitempty"`
	DegradedAttempts      int     `json:"degraded_attempts,omitempty"`
	DegradedSuccesses     int     `json:"degraded_successes,omitempty"`
	DefaultProfileCount   int     `json:"default_profile_count,omitempty"`
	NativeHeavyCount      int     `json:"native_heavy_count,omitempty"`
}

// LogChunkStats summarizes streaming throughput.
type LogChunkStats struct {
	Total int `json:"total"`
}

// Store abstracts history, hints, logs, manifests.
type Store interface {
	// Events
	Recent(ctx context.Context, limit, offset int, pkg, status string) ([]Event, error)
	History(ctx context.Context, filter HistoryFilter) ([]Event, error)
	Summary(ctx context.Context, failureLimit int) (Summary, error)
	PackageSummary(ctx context.Context, name string) (PackageSummary, error)
	LatestEvent(ctx context.Context, name, version string) (Event, error)
	Failures(ctx context.Context, name string, limit int) ([]Event, error)
	Variants(ctx context.Context, name string, limit int) ([]Event, error)
	TopFailures(ctx context.Context, limit int) ([]Stat, error)
	TopSlowest(ctx context.Context, limit int) ([]Stat, error)
	RecordEvent(ctx context.Context, evt Event) error

	// Hints
	ListHints(ctx context.Context) ([]Hint, error)
	GetHint(ctx context.Context, id string) (Hint, error)
	PutHint(ctx context.Context, hint Hint) error
	DeleteHint(ctx context.Context, id string) error

	// Logs
	GetLog(ctx context.Context, name, version string) (LogEntry, error)
	SearchLogs(ctx context.Context, q string, limit int) ([]LogEntry, error)
	PutLog(ctx context.Context, entry LogEntry) error
	PutLogChunk(ctx context.Context, chunk LogChunk) (int64, error)
	ListLogChunks(ctx context.Context, name, version string, afterID int64, afterSeq int64, attempt int, limit int) ([]LogChunk, error)
	TailLogChunks(ctx context.Context, name, version string, attempt int, limit int) ([]LogChunk, error)
	TrimLogChunks(ctx context.Context, name, version string, max int) (int64, error)
	TrimLogChunksBefore(ctx context.Context, cutoff time.Time) (int64, error)
	TrimLogsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	TrimEventsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	TrimBuildAttemptsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	TrimManifestsBefore(ctx context.Context, cutoff time.Time) (int64, error)

	// Plan/Manifest/Artifacts
	Plan(ctx context.Context) ([]PlanNode, error)
	PlanSnapshot(ctx context.Context, planID int64) (PlanSnapshot, error)
	LatestPlanSnapshot(ctx context.Context) (PlanSnapshot, error)
	ListPlans(ctx context.Context, limit int) ([]PlanSummary, error)
	SavePlan(ctx context.Context, runID string, nodes []PlanNode, dag json.RawMessage) (int64, error)
	DeletePlans(ctx context.Context, planID int64) (int64, error)
	QueueBuildsFromPlan(ctx context.Context, runID string, planID int64, nodes []PlanNode) error
	Manifest(ctx context.Context, limit int) ([]ManifestEntry, error)
	ManifestPackages(ctx context.Context, limit int) ([]string, error)
	ManifestByNormalizedName(ctx context.Context, normalized string, limit int) ([]ManifestEntry, error)
	SaveManifest(ctx context.Context, entries []ManifestEntry) error
	BuildAttemptStats(ctx context.Context, since time.Time) (BuildAttemptStats, error)
	LogChunkStats(ctx context.Context, since time.Time) (LogChunkStats, error)
	Artifacts(ctx context.Context, limit int) ([]Artifact, error)

	// Pending inputs & planning
	AddPendingInput(ctx context.Context, pi PendingInput) (int64, error)
	ListPendingInputs(ctx context.Context, status string) ([]PendingInput, error)
	PendingInputCount(ctx context.Context, status string) (int, error)
	UpdatePendingInputStatus(ctx context.Context, id int64, status, errMsg string) error
	DeletePendingInput(ctx context.Context, id int64) (PendingInput, error)
	RestorePendingInput(ctx context.Context, id int64) (PendingInput, error)
	LinkPlanToPendingInput(ctx context.Context, pendingID, planID int64) error
	UpdatePendingInputsForPlan(ctx context.Context, planID int64, status string) (int64, error)

	// Build status/queue visibility
	ListBuilds(ctx context.Context, status string, limit int, planID int64, pkg string, version string) ([]BuildStatus, error)
	BuildQueueStats(ctx context.Context) (BuildQueueStats, error)
	UpdateBuildStatus(ctx context.Context, pkg, version, status, errMsg, summary string, attempts int, backoffUntil int64, backoffReason string, backoffSeconds int, reasonCode string, reasonDetail string, recipes []string, hintIDs []string, metadata map[string]any, planID int64, nodeID string, workerID string) error
	UpsertBuildAttempt(ctx context.Context, attempt BuildAttempt) error
	ListBuildAttempts(ctx context.Context, pkg, version string, limit int) ([]BuildAttempt, error)
	LeaseBuilds(ctx context.Context, max int, workerID string) ([]BuildStatus, error)
	RequeueStaleBuilds(ctx context.Context, leaseAgeSec int, buildAgeSec int) ([]BuildStatus, error)
	DeleteBuilds(ctx context.Context, status string) (int64, error)

	// Worker health
	UpsertWorkerStatus(ctx context.Context, status WorkerStatus) error
	ListWorkers(ctx context.Context) ([]WorkerStatus, error)

	// Settings
	GetSettings(ctx context.Context) (settings.Settings, error)
	SaveSettings(ctx context.Context, s settings.Settings) error
}

// HistoryFilter defines filters for history queries.
type HistoryFilter struct {
	Package string
	Status  string
	RunID   string
	FromTs  int64
	ToTs    int64
	Limit   int
	Offset  int
}
