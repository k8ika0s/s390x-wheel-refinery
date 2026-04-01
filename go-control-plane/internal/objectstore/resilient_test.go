package objectstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/circuitbreaker"
)

// mockObjectStore implements Store for testing
type mockObjectStore struct {
	putFunc func(ctx context.Context, key string, data []byte, contentType string) error
	urlFunc func(key string) string
	calls   int
}

func (m *mockObjectStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	m.calls++
	if m.putFunc != nil {
		return m.putFunc(ctx, key, data, contentType)
	}
	return nil
}

func (m *mockObjectStore) URL(key string) string {
	if m.urlFunc != nil {
		return m.urlFunc(key)
	}
	return "http://example.com/" + key
}

func TestResilientObjectStoreSuccess(t *testing.T) {
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			return nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestResilientObjectStoreFailure(t *testing.T) {
	testErr := errors.New("test error")
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			return testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
}

func TestResilientObjectStoreCircuitBreaker(t *testing.T) {
	testErr := errors.New("test error")
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			return testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	// Trigger circuit breaker to open (5 consecutive failures)
	for i := 0; i < 5; i++ {
		_ = resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	}

	// Circuit should be open now
	if resilient.State() != circuitbreaker.StateOpen {
		t.Errorf("expected circuit to be open, got %v", resilient.State())
	}

	// Next request should be rejected
	err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestResilientObjectStoreWithRetry(t *testing.T) {
	attempts := 0
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	err := resilient.PutWithRetry(ctx, "test-key", []byte("test data"), "text/plain", 5)
	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestResilientObjectStoreWithRetryMaxRetries(t *testing.T) {
	testErr := errors.New("persistent error")
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			return testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	err := resilient.PutWithRetry(ctx, "test-key", []byte("test data"), "text/plain", 3)
	if err == nil {
		t.Error("expected error after max retries")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected error to wrap test error, got %v", err)
	}
}

func TestResilientObjectStoreContextCancellation(t *testing.T) {
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return nil
			}
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestResilientObjectStoreURL(t *testing.T) {
	mock := &mockObjectStore{
		urlFunc: func(key string) string {
			return "http://test.com/" + key
		},
	}

	resilient := NewResilientStore(mock, "test")

	url := resilient.URL("test-key")
	expected := "http://test.com/test-key"
	if url != expected {
		t.Errorf("expected URL %s, got %s", expected, url)
	}
}

func TestResilientObjectStoreCounts(t *testing.T) {
	successCount := 0
	failCount := 0
	callCount := 0
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			callCount++
			if callCount <= 3 {
				return nil
			}
			return errors.New("error")
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	// 3 successes
	for i := 0; i < 3; i++ {
		err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
		if err == nil {
			successCount++
		}
	}

	// 2 failures
	for i := 0; i < 2; i++ {
		err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
		if err != nil {
			failCount++
		}
	}

	counts := resilient.Counts()
	if counts.TotalSuccesses != uint32(successCount) {
		t.Errorf("expected %d successes, got %d", successCount, counts.TotalSuccesses)
	}
	if counts.TotalFailures != uint32(failCount) {
		t.Errorf("expected %d failures, got %d", failCount, counts.TotalFailures)
	}
}

func TestResilientObjectStoreRecovery(t *testing.T) {
	attempts := 0
	mock := &mockObjectStore{
		putFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			attempts++
			// Fail first 5 times, then succeed
			if attempts <= 5 {
				return errors.New("temporary error")
			}
			return nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()

	// Trigger circuit to open
	for i := 0; i < 5; i++ {
		_ = resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	}

	if resilient.State() != circuitbreaker.StateOpen {
		t.Errorf("expected circuit to be open, got %v", resilient.State())
	}

	// Wait for circuit to transition to half-open
	time.Sleep(time.Second * 31)

	// Next request should succeed and close the circuit
	err := resilient.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if err != nil {
		t.Errorf("expected no error after recovery, got %v", err)
	}
}

func TestNullStore(t *testing.T) {
	store := NullStore{}
	ctx := context.Background()

	// Put should succeed (no-op)
	err := store.Put(ctx, "test-key", []byte("test data"), "text/plain")
	if err != nil {
		t.Errorf("expected no error from NullStore.Put, got %v", err)
	}

	// URL should return empty string
	url := store.URL("test-key")
	if url != "" {
		t.Errorf("expected empty URL from NullStore, got %s", url)
	}
}

func TestResilientStoreState(t *testing.T) {
	mock := &mockObjectStore{}
	resilient := NewResilientStore(mock, "test")

	// Initial state should be closed
	if resilient.State() != circuitbreaker.StateClosed {
		t.Errorf("expected initial state to be closed, got %v", resilient.State())
	}
}

// Made with Bob
