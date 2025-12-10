package queue

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileQueue is a simple file-backed queue backend (JSON list).
type FileQueue struct {
	path string
	mu   sync.Mutex
}

// NewFileQueue creates a queue at the given file path.
func NewFileQueue(path string) *FileQueue {
	return &FileQueue{path: path}
}

func (f *FileQueue) load() ([]Request, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Request{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Request
	if len(data) == 0 {
		return []Request{}, nil
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (f *FileQueue) save(items []Request) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0o644)
}

func (f *FileQueue) Enqueue(ctx context.Context, req Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if req.EnqueuedAt == 0 {
		req.EnqueuedAt = time.Now().Unix()
	}
	items, err := f.load()
	if err != nil {
		return err
	}
	items = append(items, req)
	return f.save(items)
}

func (f *FileQueue) List(ctx context.Context) ([]Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return f.load()
}

func (f *FileQueue) Clear(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return f.save([]Request{})
}

func (f *FileQueue) Stats(ctx context.Context) (Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-ctx.Done():
		return Stats{}, ctx.Err()
	default:
	}
	items, err := f.load()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Length: len(items)}
	if len(items) > 0 {
		oldest := items[0].EnqueuedAt
		for _, it := range items[1:] {
			if it.EnqueuedAt < oldest {
				oldest = it.EnqueuedAt
			}
		}
		stats.OldestAge = time.Now().Unix() - oldest
	}
	return stats, nil
}

func (f *FileQueue) Pop(ctx context.Context, max int) ([]Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	items, err := f.load()
	if err != nil {
		return nil, err
	}
	if max <= 0 || max > len(items) {
		max = len(items)
	}
	toReturn := append([]Request(nil), items[:max]...)
	remaining := append([]Request(nil), items[max:]...)
	if err := f.save(remaining); err != nil {
		return nil, err
	}
	return toReturn, nil
}
