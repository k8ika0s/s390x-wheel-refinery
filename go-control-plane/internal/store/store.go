package store

import (
	"context"
)

// Event represents a build event history row.
type Event struct {
	RunID      string
	Name       string
	Version    string
	PythonTag  string
	PlatformTag string
	Status     string
	Detail     string
	Metadata   map[string]any
	Timestamp  int64
}

// Hint represents a hint catalog entry.
type Hint struct {
	ID       string
	Pattern  string
	Recipes  map[string][]string
	Note     string
}

// LogEntry represents stored log metadata/content.
type LogEntry struct {
	Name      string
	Version   string
	Content   string
	Timestamp int64
}

// Store abstracts history, hints, logs, manifests.
type Store interface {
	// Events
	Recent(ctx context.Context, limit int, pkg, status string) ([]Event, error)
	History(ctx context.Context, filter HistoryFilter) ([]Event, error)
	RecordEvent(ctx context.Context, evt Event) error

	// Hints
	ListHints(ctx context.Context) ([]Hint, error)
	GetHint(ctx context.Context, id string) (Hint, error)
	PutHint(ctx context.Context, hint Hint) error
	DeleteHint(ctx context.Context, id string) error

	// Logs
	GetLog(ctx context.Context, name, version string) (LogEntry, error)
	SearchLogs(ctx context.Context, q string, limit int) ([]LogEntry, error)
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
