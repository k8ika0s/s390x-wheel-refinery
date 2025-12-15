package cas

import (
	"context"
	"sync"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
)

// Store is a minimal content-addressable store interface used by the planner.
type Store interface {
	Has(ctx context.Context, id artifact.ID) (bool, error)
}

// NullStore always reports a miss.
type NullStore struct{}

func (NullStore) Has(_ context.Context, _ artifact.ID) (bool, error) { return false, nil }

// MemoryStore is a thread-safe in-memory store useful for tests.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]struct{}
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]struct{})}
}

func (m *MemoryStore) Has(_ context.Context, id artifact.ID) (bool, error) {
	m.mu.RLock()
	_, ok := m.items[id.Digest]
	m.mu.RUnlock()
	return ok, nil
}

// Add inserts an artifact digest for testing.
func (m *MemoryStore) Add(id artifact.ID) {
	m.mu.Lock()
	m.items[id.Digest] = struct{}{}
	m.mu.Unlock()
}
