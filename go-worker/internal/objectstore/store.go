package objectstore

import "context"

// Store uploads artifacts (e.g., wheels) to an object storage backend.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	// URL returns a fetchable URL for the given key, if available.
	URL(key string) string
}

// NullStore discards uploads.
type NullStore struct{}

func (NullStore) Put(_ context.Context, _ string, _ []byte, _ string) error { return nil }

func (NullStore) URL(_ string) string { return "" }
