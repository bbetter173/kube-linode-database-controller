package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestRetryWithBackoff(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Basic retry config with short durations for testing
	retryConfig := config.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	}

	tests := []struct {
		name          string
		fn            RetryableFunc
		isRetryable   IsRetryable
		expectSuccess bool
		expectedAttempts int
	}{
		{
			name: "Success on first attempt",
			fn: func() error {
				return nil
			},
			isRetryable:   DefaultIsRetryable,
			expectSuccess: true,
			expectedAttempts: 1,
		},
		{
			name: "Success after retries",
			fn: createCountingRetryFunc(3),
			isRetryable:   DefaultIsRetryable,
			expectSuccess: true,
			expectedAttempts: 3,
		},
		{
			name: "Failure after max retries",
			fn: func() error {
				return errors.New("persistent error")
			},
			isRetryable:   DefaultIsRetryable,
			expectSuccess: false,
			expectedAttempts: 3,
		},
		{
			name: "Non-retryable error",
			fn: func() error {
				return errors.New("non-retryable error")
			},
			isRetryable: func(err error) bool {
				return false
			},
			expectSuccess: false,
			expectedAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			wrappedFn := func() error {
				attempts++
				return tt.fn()
			}

			err := RetryWithBackoff(ctx, logger, retryConfig, "test-operation", wrappedFn, tt.isRetryable)

			if tt.expectSuccess {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expectedAttempts, attempts, "Unexpected number of attempts")
		})
	}
}

func TestRetryWithBackoffCancelContext(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	
	retryConfig := config.RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
	}

	// Create a function that will always fail, so we keep retrying
	fn := func() error {
		return errors.New("error to trigger retry")
	}

	// Cancel the context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// The retry should be interrupted by the context cancellation
	err := RetryWithBackoff(ctx, logger, retryConfig, "test-cancel", fn, DefaultIsRetryable)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operation cancelled")
}

// TestBackoffCalculation tests the backoff calculation to ensure it respects min/max values
func TestBackoffCalculation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create a test function that records backoff times
	var backoffDurations []time.Duration
	testStartTime := time.Now()

	testFn := func() error {
		if len(backoffDurations) > 0 {
			elapsed := time.Since(testStartTime)
			backoffDurations = append(backoffDurations, elapsed)
			testStartTime = time.Now()
		} else {
			backoffDurations = append(backoffDurations, 0)
		}
		return errors.New("always fail")
	}

	// Test with a specific retry config
	retryConfig := config.RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     40 * time.Millisecond,
	}

	// Run the retry function
	_ = RetryWithBackoff(ctx, logger, retryConfig, "backoff-test", testFn, DefaultIsRetryable)

	// Check the backoff progression
	assert.Len(t, backoffDurations, 4) // Should have 4 attempts
	
	// First attempt has no backoff
	assert.Less(t, backoffDurations[0], 10*time.Millisecond) // Increase tolerance
	
	// Second attempt should have around 10ms backoff
	assert.GreaterOrEqual(t, backoffDurations[1], 5*time.Millisecond)
	assert.Less(t, backoffDurations[1], 25*time.Millisecond) // Increase upper bound
	
	// Third attempt should have around 20ms backoff with jitter
	assert.GreaterOrEqual(t, backoffDurations[2], 10*time.Millisecond)
	assert.Less(t, backoffDurations[2], 35*time.Millisecond) // Increase upper bound
	
	// Fourth attempt should have around 40ms backoff (capped)
	assert.GreaterOrEqual(t, backoffDurations[3], 25*time.Millisecond)
	assert.Less(t, backoffDurations[3], 60*time.Millisecond) // Increase upper bound
}

// TestMaxBackoffCap specifically tests that backoff is capped
func TestMaxBackoffCap(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Configure with a low max backoff to test capping
	retryConfig := config.RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond, // This should cap the backoff
	}

	// We'll measure the time between attempts
	attemptTimes := make([]time.Time, 0, 5)

	testFn := func() error {
		now := time.Now()
		attemptTimes = append(attemptTimes, now)
		return errors.New("always fail")
	}

	// Run the retry function
	_ = RetryWithBackoff(ctx, logger, retryConfig, "max-backoff-test", testFn, DefaultIsRetryable)

	// Check that we have the expected number of attempts
	assert.Len(t, attemptTimes, 5)

	// Calculate intervals between attempts
	intervals := make([]time.Duration, 0, 4)
	for i := 1; i < len(attemptTimes); i++ {
		interval := attemptTimes[i].Sub(attemptTimes[i-1])
		intervals = append(intervals, interval)
	}

	// The 3rd and 4th intervals should be capped at maxBackoff
	// We allow more wiggle room for test execution delay and jitter
	for i := 2; i < len(intervals); i++ {
		assert.Less(t, intervals[i], 40*time.Millisecond, 
			"Interval %d should be capped at maxBackoff plus some reasonable tolerance", i)
	}
}

// TestZeroBackoffConfig tests behavior with zero backoff settings
func TestZeroBackoffConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Config with zero values
	retryConfig := config.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 0,
		MaxBackoff:     0,
	}

	attempts := 0
	testFn := func() error {
		attempts++
		return errors.New("always fail")
	}

	// Should still retry MaxAttempts times, even with zero backoff
	err := RetryWithBackoff(ctx, logger, retryConfig, "zero-backoff", testFn, DefaultIsRetryable)
	
	assert.Error(t, err)
	assert.Equal(t, 3, attempts)
}

func TestDefaultIsRetryable(t *testing.T) {
	// Test that the default implementation returns true for errors
	err := errors.New("some error")
	assert.True(t, DefaultIsRetryable(err))
}

// Custom error type for testing
type customError struct {
	msg       string
	retryable bool
}

// Implement the Error interface
func (e customError) Error() string {
	return e.msg
}

// TestCustomRetryPredicate tests using a custom retry predicate
func TestCustomRetryPredicate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	retryConfig := config.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	}

	// Custom retry predicate that checks our custom error
	customIsRetryable := func(err error) bool {
		if ce, ok := err.(customError); ok {
			return ce.retryable
		}
		return false
	}

	t.Run("Retryable custom error", func(t *testing.T) {
		attempts := 0
		testFn := func() error {
			attempts++
			if attempts < 3 {
				return customError{msg: "temporary error", retryable: true}
			}
			return nil
		}

		err := RetryWithBackoff(ctx, logger, retryConfig, "custom-predicate", testFn, customIsRetryable)
		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("Non-retryable custom error", func(t *testing.T) {
		attempts := 0
		testFn := func() error {
			attempts++
			return customError{msg: "permanent error", retryable: false}
		}

		err := RetryWithBackoff(ctx, logger, retryConfig, "custom-predicate", testFn, customIsRetryable)
		assert.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func BenchmarkRetryWithBackoff(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()
	
	retryConfig := config.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
	}
	
	// Function that succeeds after two attempts
	var successAfterTwo RetryableFunc = func() (err error) {
		static := &struct{ count int }{0}
		static.count++
		if static.count < 2 {
			return errors.New("temporary error")
		}
		return nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RetryWithBackoff(ctx, logger, retryConfig, "benchmark", successAfterTwo, DefaultIsRetryable)
	}
}

// Helper function to create a function that succeeds after a certain number of attempts
func createCountingRetryFunc(successAttempt int) RetryableFunc {
	counter := 0
	return func() error {
		counter++
		if counter < successAttempt {
			return errors.New("temporary error")
		}
		return nil
	}
} 