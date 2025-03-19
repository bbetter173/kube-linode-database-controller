package kubernetes

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// MinBackoffSeconds is the minimum backoff duration in seconds
	MinBackoffSeconds = 1
	
	// MaxBackoffSeconds is the maximum backoff duration in seconds
	MaxBackoffSeconds = 60
	
	// BackoffFactor is the multiplier for each retry attempt
	BackoffFactor = 2.0
	
	// BackoffJitter adds randomness to the backoff duration to prevent thundering herd
	BackoffJitter = 0.2
	
	// CircuitBreakerFailureThreshold is the number of failures before circuit breaks
	CircuitBreakerFailureThreshold = 10
	
	// CircuitBreakerWindowDuration is the sliding time window for counting failures
	CircuitBreakerWindowDuration = 5 * time.Minute
	
	// CircuitBreakerCooldownPeriod is how long to keep the circuit open before trying again
	CircuitBreakerCooldownPeriod = 60 * time.Second
)

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	// CircuitClosed means the operations can proceed normally
	CircuitClosed CircuitBreakerState = iota
	
	// CircuitOpen means the circuit is broken and operations should fail fast
	CircuitOpen
	
	// CircuitHalfOpen means we're testing if the system has recovered
	CircuitHalfOpen
)

// ReliabilityTracker implements exponential backoff and circuit breaker patterns
type ReliabilityTracker struct {
	// For exponential backoff
	retryCount      int
	lastRetryTime   time.Time
	
	// For circuit breaker
	state           CircuitBreakerState
	failuresMutex   sync.RWMutex
	failures        []time.Time
	trippedAt       time.Time
	
	logger          *zap.Logger
	exitFunc        func(int) // Function to call when circuit breaker trips
}

// NewReliabilityTracker creates a new tracker for handling retries and circuit breaking
func NewReliabilityTracker(logger *zap.Logger, exitFunc func(int)) *ReliabilityTracker {
	return &ReliabilityTracker{
		retryCount:    0,
		lastRetryTime: time.Now(),
		state:         CircuitClosed,
		failures:      make([]time.Time, 0, CircuitBreakerFailureThreshold),
		logger:        logger,
		exitFunc:      exitFunc,
	}
}

// RecordFailure tracks a failure and potentially opens the circuit
func (r *ReliabilityTracker) RecordFailure() {
	r.failuresMutex.Lock()
	defer r.failuresMutex.Unlock()
	
	now := time.Now()
	
	// Only increment retry count if we're in closed state
	if r.state == CircuitClosed {
		r.retryCount++
	}
	
	// Add the current failure
	r.failures = append(r.failures, now)
	
	// Remove failures outside our window
	cutoff := now.Add(-CircuitBreakerWindowDuration)
	newFailures := make([]time.Time, 0, len(r.failures))
	for _, t := range r.failures {
		if t.After(cutoff) {
			newFailures = append(newFailures, t)
		}
	}
	r.failures = newFailures
	
	// Check if we should open the circuit
	if r.state == CircuitClosed && len(r.failures) >= CircuitBreakerFailureThreshold {
		r.logger.Error("Circuit breaker tripped - too many failures",
			zap.Int("failures", len(r.failures)),
			zap.Duration("window", CircuitBreakerWindowDuration),
			zap.Int("exit_code", 1),
		)
		r.state = CircuitOpen
		r.trippedAt = now
		
		// In a separate goroutine, actually exit the application
		go func() {
			// Give the error time to log
			time.Sleep(500 * time.Millisecond)
			r.logger.Error("Application exiting with non-zero status code due to circuit breaker trip")
			r.exitFunc(1) // Exit with error code 1
		}()
	}
}

// RecordSuccess resets the retry count and potentially closes the circuit
func (r *ReliabilityTracker) RecordSuccess() {
	r.failuresMutex.Lock()
	defer r.failuresMutex.Unlock()
	
	r.retryCount = 0
	
	// If we're in half-open state and get success, we can close the circuit
	if r.state == CircuitHalfOpen {
		r.logger.Info("Circuit breaker reset - system recovered")
		r.state = CircuitClosed
		r.failures = make([]time.Time, 0, CircuitBreakerFailureThreshold)
	}
}

// IsCircuitOpen returns true if the circuit is open (meaning we should fail fast)
func (r *ReliabilityTracker) IsCircuitOpen() bool {
	r.failuresMutex.RLock()
	defer r.failuresMutex.RUnlock()
	
	now := time.Now()
	
	// If we're in open state but cooldown period has passed, 
	// transition to half-open to allow a test request
	if r.state == CircuitOpen && now.Sub(r.trippedAt) > CircuitBreakerCooldownPeriod {
		r.logger.Info("Circuit breaker in half-open state - testing recovery")
		r.state = CircuitHalfOpen
	}
	
	return r.state == CircuitOpen
}

// GetBackoffDuration returns the current backoff duration with jitter
func (r *ReliabilityTracker) GetBackoffDuration() time.Duration {
	r.failuresMutex.RLock()
	retryCount := r.retryCount
	r.failuresMutex.RUnlock()
	
	// Calculate exponential backoff: min(maxBackoff, minBackoff * (backoffFactor ^ retryCount))
	seconds := float64(MinBackoffSeconds) * math.Pow(BackoffFactor, float64(retryCount))
	if seconds > float64(MaxBackoffSeconds) {
		seconds = float64(MaxBackoffSeconds)
	}
	
	// Add jitter: backoff * (1 ± jitter)
	jitterAmount := 1.0 + (rand.Float64()*2*BackoffJitter - BackoffJitter)
	finalSeconds := seconds * jitterAmount
	
	return time.Duration(finalSeconds * float64(time.Second))
} 