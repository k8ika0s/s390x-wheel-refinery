package queue

import "context"

// Request is a retry/build request stored in the queue.
type Request struct {
	Package       string   `json:"package"`
	Version       string   `json:"version"`
	PythonVersion string   `json:"python_version,omitempty"`
	PythonTag     string   `json:"python_tag,omitempty"`
	PlatformTag   string   `json:"platform_tag,omitempty"`
	Recipes       []string `json:"recipes,omitempty"`
	EnqueuedAt    int64    `json:"enqueued_at,omitempty"`
	Attempts      int      `json:"attempts,omitempty"`
	PlanID        int64    `json:"plan_id,omitempty"`
	RunID         string   `json:"run_id,omitempty"`
}

// Backend defines operations for the queue.
type Backend interface {
	Enqueue(ctx context.Context, req Request) error
	List(ctx context.Context) ([]Request, error)
	Clear(ctx context.Context) error
	Stats(ctx context.Context) (Stats, error)
	Pop(ctx context.Context, max int) ([]Request, error)
}

// Stats summarizes queue depth and oldest item age.
type Stats struct {
	Length    int   `json:"length"`
	OldestAge int64 `json:"oldest_age_seconds"`
}
