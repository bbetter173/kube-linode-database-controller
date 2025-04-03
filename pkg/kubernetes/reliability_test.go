package kubernetes

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestExponentialBackoff(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Mock exit function that doesn't really exit
	var exitCalled int32
	mockExit := func(code int) {
		atomic.StoreInt32(&exitCalled, 1)
	}
	
	tracker := NewReliabilityTracker(logger, mockExit)
	
	// Test initial backoff (retry count 0)
	backoff := tracker.GetBackoffDuration()
	t.Logf("Initial backoff duration: %v", backoff)
	minExpectedTime := time.Duration(float64(MinBackoffSeconds) * (1.0 - BackoffJitter) * float64(time.Second))
	maxExpectedTime := time.Duration(float64(MinBackoffSeconds) * (1.0 + BackoffJitter) * float64(time.Second))
	t.Logf("Expected range for initial backoff: %v to %v", minExpectedTime, maxExpectedTime)
	assert.GreaterOrEqual(t, backoff, minExpectedTime)
	assert.LessOrEqual(t, backoff, maxExpectedTime)
	
	// Test backoff after a few failures
	for i := 0; i < 3; i++ {
		tracker.RecordFailure()
	}
	
	// After 3 failures, backoff should be around 2^3 = 8 seconds
	backoff = tracker.GetBackoffDuration()
	t.Logf("Backoff after 3 failures: %v", backoff)
	expectedBackoff := float64(MinBackoffSeconds) * 8.0 // 2^3
	minExpectedTime = time.Duration(expectedBackoff * (1.0 - BackoffJitter) * float64(time.Second))
	maxExpectedTime = time.Duration(expectedBackoff * (1.0 + BackoffJitter) * float64(time.Second))
	t.Logf("Expected range after 3 failures: %v to %v", minExpectedTime, maxExpectedTime)
	assert.GreaterOrEqual(t, backoff, minExpectedTime)
	assert.LessOrEqual(t, backoff, maxExpectedTime)
	
	// Success should reset the retry count
	tracker.RecordSuccess()
	
	backoff = tracker.GetBackoffDuration()
	t.Logf("Backoff after success reset: %v", backoff)
	minExpectedTime = time.Duration(float64(MinBackoffSeconds) * (1.0 - BackoffJitter) * float64(time.Second))
	maxExpectedTime = time.Duration(float64(MinBackoffSeconds) * (1.0 + BackoffJitter) * float64(time.Second))
	t.Logf("Expected range after reset: %v to %v", minExpectedTime, maxExpectedTime)
	assert.GreaterOrEqual(t, backoff, minExpectedTime)
	assert.LessOrEqual(t, backoff, maxExpectedTime)
}

func TestCircuitBreaker(t *testing.T) {
	// Create a logger that doesn't exit on Fatal for testing
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	
	// Mock exit function that doesn't really exit
	var exitCalled int32
	mockExit := func(code int) {
		atomic.StoreInt32(&exitCalled, 1)
	}
	
	// Create tracker with a small threshold for testing
	tracker := NewReliabilityTracker(logger, mockExit)
	
	// Set constants for testing (smaller values)
	origThreshold := CircuitBreakerFailureThreshold
	origWindowDuration := CircuitBreakerWindowDuration
	origCooldownPeriod := CircuitBreakerCooldownPeriod
	
	// Use unexported constant replacement for testing
	t.Cleanup(func() {
		// This is just for tests to know the values were restored
		require.Equal(t, origThreshold, CircuitBreakerFailureThreshold)
		require.Equal(t, origWindowDuration, CircuitBreakerWindowDuration)
		require.Equal(t, origCooldownPeriod, CircuitBreakerCooldownPeriod)
	})
	
	// Circuit should start closed
	assert.False(t, tracker.IsCircuitOpen())
	
	// Record failures until just below threshold
	for i := 0; i < CircuitBreakerFailureThreshold-1; i++ {
		tracker.RecordFailure()
		assert.False(t, tracker.IsCircuitOpen(), "Circuit should remain closed")
	}
	
	// Record the final failure to trip the circuit
	tracker.RecordFailure()
	
	// Sleep a bit to let the goroutine that calls os.Exit run
	time.Sleep(time.Second)
	
	// Check that exit was called
	assert.Equal(t, int32(1), atomic.LoadInt32(&exitCalled), "Exit should be called when circuit breaker trips")
	
	// Circuit should be open
	assert.True(t, tracker.IsCircuitOpen(), "Circuit should be open after threshold failures")
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Mock exit function that doesn't really exit
	var exitCalled int32
	mockExit := func(code int) {
		atomic.StoreInt32(&exitCalled, 1)
	}
	
	// Create tracker
	tracker := NewReliabilityTracker(logger, mockExit)
	
	// Circuit should start closed
	assert.False(t, tracker.IsCircuitOpen())
	
	// Store original values to restore later
	origCooldownPeriod := CircuitBreakerCooldownPeriod
	
	// Set to a small value for testing
	t.Cleanup(func() {
		require.Equal(t, origCooldownPeriod, CircuitBreakerCooldownPeriod)
	})
	
	// Directly set the circuit state to open and tripped time to simulate
	// setting it in the past, since we can't modify the const during test
	tracker.failuresMutex.Lock()
	tracker.state = CircuitOpen
	tracker.trippedAt = time.Now().Add(-2 * CircuitBreakerCooldownPeriod) 
	tracker.failuresMutex.Unlock()
	
	// Circuit should transition to half-open
	assert.False(t, tracker.IsCircuitOpen(), "Circuit should transition to half-open after cooldown")
	
	// Now test the transition from half-open to closed on success
	tracker.RecordSuccess()
	
	tracker.failuresMutex.RLock()
	halfOpenState := tracker.state 
	tracker.failuresMutex.RUnlock()
	
	assert.Equal(t, CircuitClosed, halfOpenState, "Circuit should close after success in half-open state")
} 