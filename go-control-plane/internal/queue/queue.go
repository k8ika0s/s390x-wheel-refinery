package queue

import "context"

// Request is a retry/build request stored in the queue.
type Request struct {
	Package     string
	Version     string
	PythonVersion string
	PythonTag   string
	PlatformTag string
	Recipes     []string
	EnqueuedAt  int64
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
