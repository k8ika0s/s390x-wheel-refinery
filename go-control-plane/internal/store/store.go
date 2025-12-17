package store

import (
	"context"
	"time"
)

// Event represents a build event history row.
type Event struct {
	RunID          string
	Name           string
	Version        string
	PythonTag      string
	PlatformTag    string
	Status         string
	Detail         string
	Metadata       map[string]any
	Timestamp      int64
	MatchedHintIDs []string
	DurationMS     int64
}

// Hint represents a hint catalog entry.
type Hint struct {
	ID      string
	Pattern string
	Recipes map[string][]string
	Note    string
}

// LogEntry represents stored log metadata/content.
type LogEntry struct {
	Name      string
	Version   string
	Content   string
	Timestamp int64
}

// PendingInput represents an uploaded requirements file awaiting planning.
type PendingInput struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	Digest    string    `json:"digest,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ManifestEntry tracks output wheel metadata.
type ManifestEntry struct {
	Name         string
	Version      string
	Wheel        string
	WheelURL     string
	RepairURL    string
	RepairDigest string
	RuntimeURL   string
	PackURLs     []string
	PythonTag    string
	PlatformTag  string
	Status       string
	CreatedAt    int64
}

// Artifact represents a downloadable/browsable build artifact.
type Artifact struct {
	Name    string
	Version string
	Path    string
	URL     string
}

// PlanNode describes a unit in the build plan/graph.
type PlanNode struct {
	Name          string
	Version       string
	PythonVersion string
	PythonTag     string
	PlatformTag   string
	Action        string
}

// BuildStatus tracks a build job derived from a plan.
type BuildStatus struct {
	ID           int64  `json:"id"`
	Package      string `json:"package"`
	Version      string `json:"version"`
	PythonTag    string `json:"python_tag"`
	PlatformTag  string `json:"platform_tag"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts"`
	LastError    string `json:"last_error,omitempty"`
	OldestAgeSec int64  `json:"oldest_age_seconds,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	RunID        string `json:"run_id,omitempty"`
	PlanID       int64  `json:"plan_id,omitempty"`
}

// PackageSummary aggregates status for a package.
type PackageSummary struct {
	Name         string
	StatusCounts map[string]int
	Latest       *Event
}

// Summary aggregates recent status counts and failures.
type Summary struct {
	StatusCounts map[string]int
	Failures     []Event
}

// Stat is a simple key/value for leaderboard style metrics.
type Stat struct {
	Name  string
	Value float64
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

	// Plan/Manifest/Artifacts
	Plan(ctx context.Context) ([]PlanNode, error)
	SavePlan(ctx context.Context, runID string, nodes []PlanNode) error
	Manifest(ctx context.Context, limit int) ([]ManifestEntry, error)
	SaveManifest(ctx context.Context, entries []ManifestEntry) error
	Artifacts(ctx context.Context, limit int) ([]Artifact, error)

	// Pending inputs & planning
	AddPendingInput(ctx context.Context, pi PendingInput) (int64, error)
	ListPendingInputs(ctx context.Context, status string) ([]PendingInput, error)
	UpdatePendingInputStatus(ctx context.Context, id int64, status, errMsg string) error

	// Build status/queue visibility
	ListBuilds(ctx context.Context, status string, limit int) ([]BuildStatus, error)
	UpdateBuildStatus(ctx context.Context, pkg, version, status, errMsg string, attempts int) error
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
