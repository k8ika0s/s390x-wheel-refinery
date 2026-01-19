package cas

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/circuitbreaker"
)

// mockStore implements Store for testing
type mockStore struct {
	hasFunc func(ctx context.Context, id artifact.ID) (bool, error)
	calls   int
}

func (m *mockStore) Has(ctx context.Context, id artifact.ID) (bool, error) {
	m.calls++
	if m.hasFunc != nil {
		return m.hasFunc(ctx, id)
	}
	return false, nil
}

func TestResilientStoreSuccess(t *testing.T) {
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			return true, nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	result, err := resilient.Has(ctx, id)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !result {
		t.Error("expected true result")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestResilientStoreFailure(t *testing.T) {
	testErr := errors.New("test error")
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			return false, testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	result, err := resilient.Has(ctx, id)
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
	if result {
		t.Error("expected false result")
	}
}

func TestResilientStoreCircuitBreaker(t *testing.T) {
	testErr := errors.New("test error")
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			return false, testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	// Trigger circuit breaker to open (5 consecutive failures)
	for i := 0; i < 5; i++ {
		_, _ = resilient.Has(ctx, id)
	}

	// Circuit should be open now
	if resilient.State() != circuitbreaker.StateOpen {
		t.Errorf("expected circuit to be open, got %v", resilient.State())
	}

	// Next request should be rejected
	_, err := resilient.Has(ctx, id)
	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestResilientStoreWithRetry(t *testing.T) {
	attempts := 0
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			attempts++
			if attempts < 3 {
				return false, errors.New("temporary error")
			}
			return true, nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	result, err := resilient.HasWithRetry(ctx, id, 5)
	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if !result {
		t.Error("expected true result after retries")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestResilientStoreWithRetryMaxRetries(t *testing.T) {
	testErr := errors.New("persistent error")
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			return false, testErr
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	_, err := resilient.HasWithRetry(ctx, id, 3)
	if err == nil {
		t.Error("expected error after max retries")
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected error to wrap test error, got %v", err)
	}
}

func TestResilientStoreContextCancellation(t *testing.T) {
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(time.Second):
				return true, nil
			}
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	id := artifact.ID{Digest: "sha256:abc123"}

	_, err := resilient.Has(ctx, id)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestResilientStoreCounts(t *testing.T) {
	successCount := 0
	failCount := 0
	callCount := 0
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			callCount++
			if callCount <= 3 {
				return true, nil
			}
			return false, errors.New("error")
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	// 3 successes
	for i := 0; i < 3; i++ {
		_, err := resilient.Has(ctx, id)
		if err == nil {
			successCount++
		}
	}

	// 2 failures
	for i := 0; i < 2; i++ {
		_, err := resilient.Has(ctx, id)
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

func TestResilientPusherSuccess(t *testing.T) {
	pusher := &Pusher{
		BaseURL: "http://localhost:5000",
		Repo:    "test",
	}

	resilient := NewResilientPusher(pusher, "test")

	// Test that resilient pusher is created successfully
	if resilient == nil {
		t.Error("expected resilient pusher to be created")
	}
	if resilient.State() != circuitbreaker.StateClosed {
		t.Errorf("expected initial state to be closed, got %v", resilient.State())
	}
}

func TestResilientFetcherSuccess(t *testing.T) {
	fetcher := &Fetcher{
		BaseURL: "http://localhost:5000",
		Repo:    "test",
	}

	resilient := NewResilientFetcher(fetcher, "test")

	// Test that resilient fetcher is created successfully
	if resilient == nil {
		t.Error("expected resilient fetcher to be created")
	}
	if resilient.State() != circuitbreaker.StateClosed {
		t.Errorf("expected initial state to be closed, got %v", resilient.State())
	}
}

func TestResilientZotStoreCreation(t *testing.T) {
	resilient := NewResilientZotStore(
		"http://localhost:5000",
		"test-repo",
		"user",
		"pass",
	)

	if resilient == nil {
		t.Error("expected resilient zot store to be created")
	}
	if resilient.zot == nil {
		t.Error("expected underlying zot store to be set")
	}
	if resilient.zot.BaseURL != "http://localhost:5000" {
		t.Errorf("expected base URL to be set, got %s", resilient.zot.BaseURL)
	}
	if resilient.zot.Repo != "test-repo" {
		t.Errorf("expected repo to be set, got %s", resilient.zot.Repo)
	}
}

func TestResilientZotPusherCreation(t *testing.T) {
	resilient := NewResilientZotPusher(
		"http://localhost:5000",
		"test-repo",
		"user",
		"pass",
	)

	if resilient == nil {
		t.Error("expected resilient zot pusher to be created")
	}
	if resilient.pusher == nil {
		t.Error("expected underlying pusher to be set")
	}
	if resilient.pusher.BaseURL != "http://localhost:5000" {
		t.Errorf("expected base URL to be set, got %s", resilient.pusher.BaseURL)
	}
}

func TestResilientZotFetcherCreation(t *testing.T) {
	resilient := NewResilientZotFetcher(
		"http://localhost:5000",
		"test-repo",
		"user",
		"pass",
	)

	if resilient == nil {
		t.Error("expected resilient zot fetcher to be created")
	}
	if resilient.fetcher == nil {
		t.Error("expected underlying fetcher to be set")
	}
	if resilient.fetcher.BaseURL != "http://localhost:5000" {
		t.Errorf("expected base URL to be set, got %s", resilient.fetcher.BaseURL)
	}
}

func TestResilientStoreRecovery(t *testing.T) {
	attempts := 0
	mock := &mockStore{
		hasFunc: func(ctx context.Context, id artifact.ID) (bool, error) {
			attempts++
			// Fail first 5 times, then succeed
			if attempts <= 5 {
				return false, errors.New("temporary error")
			}
			return true, nil
		},
	}

	resilient := NewResilientStore(mock, "test")
	ctx := context.Background()
	id := artifact.ID{Digest: "sha256:abc123"}

	// Trigger circuit to open
	for i := 0; i < 5; i++ {
		_, _ = resilient.Has(ctx, id)
	}

	if resilient.State() != circuitbreaker.StateOpen {
		t.Errorf("expected circuit to be open, got %v", resilient.State())
	}

	// Wait for circuit to transition to half-open
	time.Sleep(time.Second * 31)

	// Next request should succeed and close the circuit
	result, err := resilient.Has(ctx, id)
	if err != nil {
		t.Errorf("expected no error after recovery, got %v", err)
	}
	if !result {
		t.Error("expected true result after recovery")
	}
}

// Made with Bob
