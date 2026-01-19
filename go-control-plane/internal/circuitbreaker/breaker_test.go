package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerStateClosed(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		Interval:    time.Millisecond * 100,
		Timeout:     time.Millisecond * 200,
	})

	if cb.State() != StateClosed {
		t.Errorf("expected state to be closed, got %v", cb.State())
	}

	// Successful request should keep circuit closed
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to be closed after success, got %v", cb.State())
	}
}

func TestCircuitBreakerStateOpen(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		Interval:    time.Millisecond * 100,
		Timeout:     time.Millisecond * 200,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	})

	// Trigger 3 failures to open circuit
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error {
			return errors.New("test error")
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open after failures, got %v", cb.State())
	}

	// Next request should be rejected
	err := cb.Execute(func() error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerStateHalfOpen(t *testing.T) {
	timeout := time.Millisecond * 100
	cb := New(Config{
		Name:        "test",
		MaxRequests: 2,
		Interval:    time.Millisecond * 50,
		Timeout:     timeout,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("test error")
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open, got %v", cb.State())
	}

	// Wait for timeout to transition to half-open
	time.Sleep(timeout + time.Millisecond*10)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state to be half-open after timeout, got %v", cb.State())
	}

	// First request in half-open should be allowed
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error in half-open, got %v", err)
	}

	// Second request should be allowed (MaxRequests=2)
	err = cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error for second request, got %v", err)
	}

	// After MaxRequests successes, should transition to closed
	if cb.State() != StateClosed {
		t.Errorf("expected state to be closed after successful half-open requests, got %v", cb.State())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	timeout := time.Millisecond * 100
	cb := New(Config{
		Name:        "test",
		MaxRequests: 2,
		Timeout:     timeout,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("test error")
		})
	}

	// Wait for half-open
	time.Sleep(timeout + time.Millisecond*10)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state to be half-open, got %v", cb.State())
	}

	// Failure in half-open should reopen circuit
	_ = cb.Execute(func() error {
		return errors.New("test error")
	})

	if cb.State() != StateOpen {
		t.Errorf("expected state to be open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreakerTooManyRequests(t *testing.T) {
	timeout := time.Millisecond * 100
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     timeout,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("test error")
		})
	}

	// Wait for half-open
	time.Sleep(timeout + time.Millisecond*10)

	// First request should be allowed
	_ = cb.Execute(func() error {
		return nil
	})

	// Second request should be rejected (MaxRequests=1)
	err := cb.Execute(func() error {
		return nil
	})
	if !errors.Is(err, ErrTooManyRequests) {
		t.Errorf("expected ErrTooManyRequests, got %v", err)
	}
}

func TestCircuitBreakerCounts(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 10,
	})

	// Execute some successful requests
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error {
			return nil
		})
	}

	counts := cb.Counts()
	if counts.Requests != 5 {
		t.Errorf("expected 5 requests, got %d", counts.Requests)
	}
	if counts.TotalSuccesses != 5 {
		t.Errorf("expected 5 successes, got %d", counts.TotalSuccesses)
	}
	if counts.ConsecutiveSuccesses != 5 {
		t.Errorf("expected 5 consecutive successes, got %d", counts.ConsecutiveSuccesses)
	}

	// Execute a failure
	_ = cb.Execute(func() error {
		return errors.New("test error")
	})

	counts = cb.Counts()
	if counts.Requests != 6 {
		t.Errorf("expected 6 requests, got %d", counts.Requests)
	}
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", counts.TotalFailures)
	}
	if counts.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", counts.ConsecutiveFailures)
	}
	if counts.ConsecutiveSuccesses != 0 {
		t.Errorf("expected 0 consecutive successes after failure, got %d", counts.ConsecutiveSuccesses)
	}
}

func TestCircuitBreakerExecuteContext(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
	})

	ctx := context.Background()

	// Successful execution
	err := cb.ExecuteContext(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Failed execution
	testErr := errors.New("test error")
	err = cb.ExecuteContext(ctx, func(ctx context.Context) error {
		return testErr
	})
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
}

func TestCircuitBreakerExecuteContextCancellation(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := cb.ExecuteContext(ctx, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return nil
		}
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCircuitBreakerStateChange(t *testing.T) {
	var stateChanges []string
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		Timeout:     time.Millisecond * 100,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
		OnStateChange: func(name string, from State, to State) {
			stateChanges = append(stateChanges, from.String()+"->"+to.String())
		},
	})

	// Trigger state change to open
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("test error")
		})
	}

	if len(stateChanges) != 1 || stateChanges[0] != "closed->open" {
		t.Errorf("expected closed->open transition, got %v", stateChanges)
	}

	// Wait for half-open
	time.Sleep(time.Millisecond * 110)
	_ = cb.State() // Trigger state check

	if len(stateChanges) != 2 || stateChanges[1] != "open->half-open" {
		t.Errorf("expected open->half-open transition, got %v", stateChanges)
	}
}

func TestCircuitBreakerCustomIsSuccessful(t *testing.T) {
	customErr := errors.New("custom error")
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
		IsSuccessful: func(err error) bool {
			// Treat customErr as success
			return err == nil || errors.Is(err, customErr)
		},
	})

	// Execute with custom error (should be treated as success)
	err := cb.Execute(func() error {
		return customErr
	})
	if err != customErr {
		t.Errorf("expected custom error to be returned, got %v", err)
	}

	counts := cb.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success (custom error treated as success), got %d", counts.TotalSuccesses)
	}

	// Circuit should still be closed
	if cb.State() != StateClosed {
		t.Errorf("expected state to be closed, got %v", cb.State())
	}
}

func TestExponentialBackoff(t *testing.T) {
	eb := NewExponentialBackoff(time.Millisecond*10, time.Millisecond*100)

	// First backoff should be around initial interval
	first := eb.Next()
	if first < time.Millisecond*5 || first > time.Millisecond*15 {
		t.Errorf("expected first backoff around 10ms, got %v", first)
	}

	// Second should be larger
	second := eb.Next()
	if second < time.Millisecond*10 || second > time.Millisecond*30 {
		t.Errorf("expected second backoff around 20ms, got %v", second)
	}

	// Should eventually cap at max
	for i := 0; i < 10; i++ {
		eb.Next()
	}
	capped := eb.Next()
	if capped > time.Millisecond*150 {
		t.Errorf("expected backoff to be capped at ~100ms, got %v", capped)
	}

	// Reset should go back to initial
	eb.Reset()
	reset := eb.Next()
	if reset < time.Millisecond*5 || reset > time.Millisecond*15 {
		t.Errorf("expected reset backoff around 10ms, got %v", reset)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	ctx := context.Background()
	backoff := NewExponentialBackoff(time.Millisecond, time.Millisecond*10)

	attempts := 0
	err := RetryWithBackoff(ctx, 3, backoff, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoffMaxRetries(t *testing.T) {
	ctx := context.Background()
	backoff := NewExponentialBackoff(time.Millisecond, time.Millisecond*10)

	attempts := 0
	testErr := errors.New("persistent error")
	err := RetryWithBackoff(ctx, 3, backoff, func() error {
		attempts++
		return testErr
	})

	if err == nil {
		t.Error("expected error after max retries")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if !errors.Is(err, testErr) {
		t.Errorf("expected error to wrap test error, got %v", err)
	}
}

func TestRetryWithBackoffContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backoff := NewExponentialBackoff(time.Millisecond*50, time.Millisecond*100)

	attempts := 0
	go func() {
		time.Sleep(time.Millisecond * 10)
		cancel()
	}()

	err := RetryWithBackoff(ctx, 10, backoff, func() error {
		attempts++
		return errors.New("error")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if attempts >= 10 {
		t.Errorf("expected fewer than 10 attempts due to cancellation, got %d", attempts)
	}
}

func TestCircuitBreakerPanic(t *testing.T) {
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to be propagated")
		}
	}()

	_ = cb.Execute(func() error {
		panic("test panic")
	})
}

func TestCircuitBreakerInterval(t *testing.T) {
	interval := time.Millisecond * 50
	cb := New(Config{
		Name:        "test",
		MaxRequests: 1,
		Interval:    interval,
	})

	// Execute a failure
	_ = cb.Execute(func() error {
		return errors.New("test error")
	})

	counts := cb.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", counts.TotalFailures)
	}

	// Wait for interval to pass
	time.Sleep(interval + time.Millisecond*10)

	// Execute another request to trigger interval reset
	_ = cb.Execute(func() error {
		return nil
	})

	counts = cb.Counts()
	// After interval, counts should be reset
	if counts.TotalFailures != 0 {
		t.Errorf("expected failures to be reset after interval, got %d", counts.TotalFailures)
	}
}

// Made with Bob
