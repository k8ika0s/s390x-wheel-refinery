package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrTooManyRequests is returned when the circuit breaker is half-open and at capacity
	ErrTooManyRequests = errors.New("circuit breaker: too many requests")

	breakerStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Current state of the circuit breaker (0=closed, 1=open, 2=half-open)",
	}, []string{"name"})

	breakerRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_requests_total",
		Help: "Total number of requests through the circuit breaker",
	}, []string{"name", "result"})

	breakerErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_errors_total",
		Help: "Total number of errors in the circuit breaker",
	}, []string{"name", "type"})

	breakerStateChangesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "circuit_breaker_state_changes_total",
		Help: "Total number of state changes",
	}, []string{"name", "from", "to"})
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed allows all requests through
	StateClosed State = iota
	// StateOpen blocks all requests
	StateOpen
	// StateHalfOpen allows limited requests to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// Name identifies this circuit breaker for metrics
	Name string

	// MaxRequests is the maximum number of requests allowed in half-open state
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for clearing internal counts
	// If Interval is 0, the circuit breaker doesn't clear internal counts during the closed state
	Interval time.Duration

	// Timeout is the period of the open state, after which the state becomes half-open
	Timeout time.Duration

	// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state
	// If ReadyToTrip returns true, the circuit breaker will be placed into the open state
	// If ReadyToTrip is nil, default threshold is used (5 consecutive failures)
	ReadyToTrip func(counts Counts) bool

	// OnStateChange is called whenever the state changes
	OnStateChange func(name string, from State, to State)

	// IsSuccessful determines if a response should be considered successful
	// If nil, only errors are considered failures
	IsSuccessful func(err error) bool
}

// Counts holds the numbers of requests and their successes/failures
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

func (c *Counts) onRequest() {
	c.Requests++
}

func (c *Counts) onSuccess() {
	c.TotalSuccesses++
	c.ConsecutiveSuccesses++
	c.ConsecutiveFailures = 0
}

func (c *Counts) onFailure() {
	c.TotalFailures++
	c.ConsecutiveFailures++
	c.ConsecutiveSuccesses = 0
}

func (c *Counts) clear() {
	c.Requests = 0
	c.TotalSuccesses = 0
	c.TotalFailures = 0
	c.ConsecutiveSuccesses = 0
	c.ConsecutiveFailures = 0
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	readyToTrip   func(counts Counts) bool
	isSuccessful  func(err error) bool
	onStateChange func(name string, from State, to State)

	mutex      sync.Mutex
	state      State
	generation uint64
	counts     Counts
	expiry     time.Time

	// Metrics
	stateGauge        prometheus.Gauge
	requestsTotal     *prometheus.CounterVec
	errorsTotal       *prometheus.CounterVec
	stateChangesTotal *prometheus.CounterVec
}

// New creates a new CircuitBreaker
func New(config Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:          config.Name,
		maxRequests:   config.MaxRequests,
		interval:      config.Interval,
		timeout:       config.Timeout,
		readyToTrip:   config.ReadyToTrip,
		isSuccessful:  config.IsSuccessful,
		onStateChange: config.OnStateChange,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}

	if cb.readyToTrip == nil {
		cb.readyToTrip = func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 5
		}
	}

	if cb.isSuccessful == nil {
		cb.isSuccessful = func(err error) bool {
			return err == nil
		}
	}

	// Shared metric vectors avoid duplicate collector registration across breaker instances.
	cb.stateGauge = breakerStateGauge.WithLabelValues(cb.name)
	cb.requestsTotal = breakerRequestsTotal.MustCurryWith(prometheus.Labels{"name": cb.name})
	cb.errorsTotal = breakerErrorsTotal.MustCurryWith(prometheus.Labels{"name": cb.name})
	cb.stateChangesTotal = breakerStateChangesTotal.MustCurryWith(prometheus.Labels{"name": cb.name})

	cb.toNewGeneration(time.Now())

	return cb
}

// Execute runs the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		cb.errorsTotal.WithLabelValues("rejected").Inc()
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn()
	cb.afterRequest(generation, cb.isSuccessful(err))
	return err
}

// ExecuteContext runs the given function with context if the circuit breaker allows it
func (cb *CircuitBreaker) ExecuteContext(ctx context.Context, fn func(context.Context) error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		cb.errorsTotal.WithLabelValues("rejected").Inc()
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn(ctx)
	cb.afterRequest(generation, cb.isSuccessful(err))
	return err
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns a copy of the current counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	return cb.counts
}

func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.maxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.onRequest()
	cb.requestsTotal.WithLabelValues("attempted").Inc()
	return generation, nil
}

func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)
	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
		cb.requestsTotal.WithLabelValues("success").Inc()
	} else {
		cb.onFailure(state, now)
		cb.requestsTotal.WithLabelValues("failure").Inc()
	}
}

func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.onSuccess()

	if state == StateHalfOpen && cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
		cb.setState(StateClosed, now)
	}
}

func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	cb.counts.onFailure()

	switch state {
	case StateClosed:
		if cb.readyToTrip(cb.counts) {
			cb.setState(StateOpen, now)
		}
	case StateHalfOpen:
		cb.setState(StateOpen, now)
	}
}

func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}

	cb.stateGauge.Set(float64(state))
	cb.stateChangesTotal.WithLabelValues(prev.String(), state.String()).Inc()
}

func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts.clear()

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// ExponentialBackoff implements exponential backoff with jitter
type ExponentialBackoff struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	RandomFactor    float64

	currentInterval time.Duration
	mutex           sync.Mutex
}

// NewExponentialBackoff creates a new exponential backoff strategy
func NewExponentialBackoff(initial, max time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialInterval: initial,
		MaxInterval:     max,
		Multiplier:      2.0,
		RandomFactor:    0.5,
		currentInterval: initial,
	}
}

// Next returns the next backoff duration
func (eb *ExponentialBackoff) Next() time.Duration {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()

	interval := eb.currentInterval

	// Add jitter
	delta := eb.RandomFactor * float64(interval)
	minInterval := float64(interval) - delta
	maxInterval := float64(interval) + delta
	interval = time.Duration(minInterval + (rand.Float64() * (maxInterval - minInterval)))

	// Increase for next time
	eb.currentInterval = time.Duration(math.Min(
		float64(eb.currentInterval)*eb.Multiplier,
		float64(eb.MaxInterval),
	))

	return interval
}

// Reset resets the backoff to initial interval
func (eb *ExponentialBackoff) Reset() {
	eb.mutex.Lock()
	defer eb.mutex.Unlock()
	eb.currentInterval = eb.InitialInterval
}

// RetryWithBackoff retries a function with exponential backoff
func RetryWithBackoff(ctx context.Context, maxRetries int, backoff *ExponentialBackoff, fn func() error) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if err := fn(); err == nil {
			backoff.Reset()
			return nil
		} else {
			lastErr = err
		}

		if i < maxRetries-1 {
			wait := backoff.Next()
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled: %w", ctx.Err())
			case <-time.After(wait):
				// Continue to next retry
			}
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

// Made with Bob
