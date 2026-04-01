package cas

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/circuitbreaker"
)

// ResilientStore wraps a CAS Store with circuit breaker protection
type ResilientStore struct {
	store   Store
	breaker *circuitbreaker.CircuitBreaker
	backoff *circuitbreaker.ExponentialBackoff
}

// NewResilientStore creates a new resilient CAS store with circuit breaker
func NewResilientStore(store Store, name string) *ResilientStore {
	breaker := circuitbreaker.New(circuitbreaker.Config{
		Name:        fmt.Sprintf("cas-%s", name),
		MaxRequests: 3,
		Interval:    time.Minute,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Open circuit after 5 consecutive failures
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from circuitbreaker.State, to circuitbreaker.State) {
			log.Printf("CAS circuit breaker %s: %s -> %s", name, from, to)
		},
		IsSuccessful: func(err error) bool {
			// Treat 404 as success (artifact not found is valid)
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

// Has checks if an artifact exists with circuit breaker protection
func (r *ResilientStore) Has(ctx context.Context, id artifact.ID) (bool, error) {
	var result bool
	var err error

	// Try with circuit breaker
	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		result, err = r.store.Has(ctx, id)
		return err
	})

	// If circuit breaker rejected the request
	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return false, fmt.Errorf("CAS unavailable: %w", cbErr)
	}

	return result, err
}

// HasWithRetry checks if an artifact exists with retry logic
func (r *ResilientStore) HasWithRetry(ctx context.Context, id artifact.ID, maxRetries int) (bool, error) {
	var result bool

	err := circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		var hasErr error
		result, hasErr = r.Has(ctx, id)
		return hasErr
	})

	return result, err
}

// State returns the current circuit breaker state
func (r *ResilientStore) State() circuitbreaker.State {
	return r.breaker.State()
}

// Counts returns the current circuit breaker counts
func (r *ResilientStore) Counts() circuitbreaker.Counts {
	return r.breaker.Counts()
}

// ResilientPusher wraps a Pusher with circuit breaker protection
type ResilientPusher struct {
	pusher  *Pusher
	breaker *circuitbreaker.CircuitBreaker
	backoff *circuitbreaker.ExponentialBackoff
}

// NewResilientPusher creates a new resilient pusher with circuit breaker
func NewResilientPusher(pusher *Pusher, name string) *ResilientPusher {
	breaker := circuitbreaker.New(circuitbreaker.Config{
		Name:        fmt.Sprintf("cas-pusher-%s", name),
		MaxRequests: 2,
		Interval:    time.Minute,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Open circuit after 3 consecutive failures
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from circuitbreaker.State, to circuitbreaker.State) {
			log.Printf("CAS pusher circuit breaker %s: %s -> %s", name, from, to)
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	})

	backoff := circuitbreaker.NewExponentialBackoff(
		time.Millisecond*200,
		time.Second*15,
	)

	return &ResilientPusher{
		pusher:  pusher,
		breaker: breaker,
		backoff: backoff,
	}
}

// Push uploads content with circuit breaker protection
func (r *ResilientPusher) Push(ctx context.Context, id artifact.ID, content []byte, mediaType string) (string, error) {
	var result string
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		result, err = r.pusher.Push(ctx, id, content, mediaType)
		return err
	})

	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return "", fmt.Errorf("CAS pusher unavailable: %w", cbErr)
	}

	return result, err
}

// PushWithRetry uploads content with retry logic
func (r *ResilientPusher) PushWithRetry(ctx context.Context, id artifact.ID, content []byte, mediaType string, maxRetries int) (string, error) {
	var result string

	err := circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		var pushErr error
		result, pushErr = r.Push(ctx, id, content, mediaType)
		return pushErr
	})

	return result, err
}

// State returns the current circuit breaker state
func (r *ResilientPusher) State() circuitbreaker.State {
	return r.breaker.State()
}

// ResilientFetcher wraps a Fetcher with circuit breaker protection
type ResilientFetcher struct {
	fetcher *Fetcher
	breaker *circuitbreaker.CircuitBreaker
	backoff *circuitbreaker.ExponentialBackoff
}

// NewResilientFetcher creates a new resilient fetcher with circuit breaker
func NewResilientFetcher(fetcher *Fetcher, name string) *ResilientFetcher {
	breaker := circuitbreaker.New(circuitbreaker.Config{
		Name:        fmt.Sprintf("cas-fetcher-%s", name),
		MaxRequests: 3,
		Interval:    time.Minute,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Open circuit after 5 consecutive failures
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from circuitbreaker.State, to circuitbreaker.State) {
			log.Printf("CAS fetcher circuit breaker %s: %s -> %s", name, from, to)
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	})

	backoff := circuitbreaker.NewExponentialBackoff(
		time.Millisecond*200,
		time.Second*15,
	)

	return &ResilientFetcher{
		fetcher: fetcher,
		breaker: breaker,
		backoff: backoff,
	}
}

// Fetch downloads an artifact with circuit breaker protection
func (r *ResilientFetcher) Fetch(ctx context.Context, id artifact.ID, destPath string) error {
	var err error

	cbErr := r.breaker.ExecuteContext(ctx, func(ctx context.Context) error {
		err = r.fetcher.Fetch(ctx, id, destPath)
		return err
	})

	if cbErr == circuitbreaker.ErrCircuitOpen || cbErr == circuitbreaker.ErrTooManyRequests {
		return fmt.Errorf("CAS fetcher unavailable: %w", cbErr)
	}

	return err
}

// FetchWithRetry downloads an artifact with retry logic
func (r *ResilientFetcher) FetchWithRetry(ctx context.Context, id artifact.ID, destPath string, maxRetries int) error {
	return circuitbreaker.RetryWithBackoff(ctx, maxRetries, r.backoff, func() error {
		return r.Fetch(ctx, id, destPath)
	})
}

// State returns the current circuit breaker state
func (r *ResilientFetcher) State() circuitbreaker.State {
	return r.breaker.State()
}

// ResilientZotStore wraps ZotStore with additional HTTP-level resilience
type ResilientZotStore struct {
	*ResilientStore
	zot *ZotStore
}

// NewResilientZotStore creates a resilient Zot store with custom HTTP client
func NewResilientZotStore(baseURL, repo, username, password string) *ResilientZotStore {
	// Create HTTP client with reasonable timeouts and retry transport
	client := &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     time.Second * 90,
			DisableKeepAlives:   false,
		},
	}

	zot := &ZotStore{
		BaseURL:  baseURL,
		Repo:     repo,
		Username: username,
		Password: password,
		Client:   client,
	}

	resilient := NewResilientStore(zot, "zot")

	return &ResilientZotStore{
		ResilientStore: resilient,
		zot:            zot,
	}
}

// NewResilientZotPusher creates a resilient Zot pusher
func NewResilientZotPusher(baseURL, repo, username, password string) *ResilientPusher {
	client := &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     time.Second * 90,
		},
	}

	pusher := &Pusher{
		BaseURL:  baseURL,
		Repo:     repo,
		Username: username,
		Password: password,
		Client:   client,
	}

	return NewResilientPusher(pusher, "zot")
}

// NewResilientZotFetcher creates a resilient Zot fetcher
func NewResilientZotFetcher(baseURL, repo, username, password string) *ResilientFetcher {
	client := &http.Client{
		Timeout: time.Second * 20,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     time.Second * 90,
		},
	}

	fetcher := &Fetcher{
		BaseURL:  baseURL,
		Repo:     repo,
		Username: username,
		Password: password,
		Client:   client,
	}

	return NewResilientFetcher(fetcher, "zot")
}

// Made with Bob
