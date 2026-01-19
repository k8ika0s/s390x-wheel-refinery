package objectstore

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/circuitbreaker"
	"github.com/minio/minio-go/v7"
)

// ResilientStore wraps an object Store with circuit breaker protection
type ResilientStore struct {
	store   Store
	breaker *circuitbreaker.CircuitBreaker
	backoff *circuitbreaker.ExponentialBackoff
}

// NewResilientStore creates a new resilient object store with circuit breaker
func NewResilientStore(store Store, name string) *ResilientStore {
	breaker := circuitbreaker.New(circuitbreaker.Config{
		Name:        fmt.Sprintf("objectstore-%s", name),
		MaxRequests: 3,
		Interval:    time.Minute,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Open circuit after 5 consecutive failures
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from circuitbreaker.State, to circuitbreaker.State) {
			log.Printf("Object store circuit breaker %s: %s -> %s", name, from, to)
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	})

	backoff := circuitbreaker.NewExponentialBackoff(
		time.Millisecond*100,
		time.Second*10,
	)

	return &ResilientStore{
		store:   store,
		breaker: breaker,
		backoff: backoff,
	}
}

// Put uploads data with circuit breaker protection
func (r *ResilientStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		err = r.store.Put(ctx, key, data, contentType)
		return err
	})

	// If circuit breaker rejected the request
	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return fmt.Errorf("object store unavailable: %w", cbErr)
	}

	return err
}

// PutWithRetry uploads data with retry logic
func (r *ResilientStore) PutWithRetry(ctx context.Context, key string, data []byte, contentType string, maxRetries int) error {
	return circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		return r.Put(ctx, key, data, contentType)
	})
}

// URL returns the URL for a key
func (r *ResilientStore) URL(key string) string {
	return r.store.URL(key)
}

// State returns the current circuit breaker state
func (r *ResilientStore) State() circuitbreaker.State {
	return r.breaker.State()
}

// Counts returns the current circuit breaker counts
func (r *ResilientStore) Counts() circuitbreaker.Counts {
	return r.breaker.Counts()
}

// ResilientMinIOStore wraps MinIOStore with additional resilience features
type ResilientMinIOStore struct {
	*ResilientStore
	minio *MinIOStore
}

// NewResilientMinIOStore creates a resilient MinIO store with circuit breaker
func NewResilientMinIOStore(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*ResilientMinIOStore, error) {
	// Create the underlying MinIO store
	minio, err := NewMinIOStore(endpoint, accessKey, secretKey, bucket, useSSL)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO store: %w", err)
	}

	// Wrap with resilient store
	resilient := NewResilientStore(minio, "minio")

	return &ResilientMinIOStore{
		ResilientStore: resilient,
		minio:          minio,
	}, nil
}

// GetUnderlyingStore returns the underlying MinIO store for advanced operations
func (r *ResilientMinIOStore) GetUnderlyingStore() *MinIOStore {
	return r.minio
}

// HealthCheck performs a health check on the MinIO store
func (r *ResilientMinIOStore) HealthCheck(ctx context.Context) error {
	// Try to list buckets as a health check
	if r.minio == nil || r.minio.Client == nil {
		return fmt.Errorf("MinIO client not initialized")
	}

	exists, err := r.minio.Client.BucketExists(ctx, r.minio.Bucket)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", r.minio.Bucket)
	}

	return nil
}

// PutWithHealthCheck uploads data after verifying store health
func (r *ResilientMinIOStore) PutWithHealthCheck(ctx context.Context, key string, data []byte, contentType string) error {
	// Check health first
	if err := r.HealthCheck(ctx); err != nil {
		return fmt.Errorf("health check failed before put: %w", err)
	}

	return r.Put(ctx, key, data, contentType)
}

// Get downloads an object from MinIO (not in base Store interface)
func (r *ResilientMinIOStore) Get(ctx context.Context, key string) ([]byte, error) {
	var data []byte
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		obj, getErr := r.minio.Client.GetObject(ctx, r.minio.Bucket, key, minio.GetObjectOptions{})
		if getErr != nil {
			return getErr
		}
		defer obj.Close()

		// Read all data
		buf := make([]byte, 0, 1024*1024) // Start with 1MB buffer
		tmp := make([]byte, 4096)
		for {
			n, readErr := obj.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if readErr != nil {
				if readErr.Error() == "EOF" {
					break
				}
				return readErr
			}
		}
		data = buf
		return nil
	})

	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return nil, fmt.Errorf("object store unavailable: %w", cbErr)
	}

	return data, err
}

// GetWithRetry downloads an object with retry logic
func (r *ResilientMinIOStore) GetWithRetry(ctx context.Context, key string, maxRetries int) ([]byte, error) {
	var data []byte

	err := circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		var getErr error
		data, getErr = r.Get(ctx, key)
		return getErr
	})

	return data, err
}

// Delete removes an object from MinIO
func (r *ResilientMinIOStore) Delete(ctx context.Context, key string) error {
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		err = r.minio.Client.RemoveObject(ctx, r.minio.Bucket, key, minio.RemoveObjectOptions{})
		return err
	})

	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return fmt.Errorf("object store unavailable: %w", cbErr)
	}

	return err
}

// DeleteWithRetry removes an object with retry logic
func (r *ResilientMinIOStore) DeleteWithRetry(ctx context.Context, key string, maxRetries int) error {
	return circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		return r.Delete(ctx, key)
	})
}

// List returns a list of objects with a given prefix
func (r *ResilientMinIOStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		objectCh := r.minio.Client.ListObjects(ctx, r.minio.Bucket, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		})

		for object := range objectCh {
			if object.Err != nil {
				return object.Err
			}
			keys = append(keys, object.Key)
		}
		return nil
	})

	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return nil, fmt.Errorf("object store unavailable: %w", cbErr)
	}

	return keys, err
}

// ListWithRetry returns a list of objects with retry logic
func (r *ResilientMinIOStore) ListWithRetry(ctx context.Context, prefix string, maxRetries int) ([]string, error) {
	var keys []string

	err := circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		var listErr error
		keys, listErr = r.List(ctx, prefix)
		return listErr
	})

	return keys, err
}

// Made with Bob
