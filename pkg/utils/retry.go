package utils

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"go.uber.org/zap"
)

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// IsRetryable determines if an error should be retried
type IsRetryable func(error) bool

// RetryWithBackoff retries a function with exponential backoff based on config
func RetryWithBackoff(ctx context.Context, logger *zap.Logger, retryConfig config.RetryConfig, operation string, fn RetryableFunc, isRetryable IsRetryable) error {
	var err error
	var nextBackoff time.Duration

	for attempt := 0; attempt < retryConfig.MaxAttempts; attempt++ {
		// Execute the function
		err = fn()
		
		// If no error or error is not retryable, return immediately
		if err == nil {
			return nil
		}
		
		if isRetryable != nil && !isRetryable(err) {
			logger.Error("Non-retryable error encountered",
				zap.String("operation", operation),
				zap.Error(err),
				zap.Int("attempt", attempt+1),
			)
			return err
		}
		
		// Check context before sleeping
		if ctx.Err() != nil {
			logger.Warn("Context cancelled during retry",
				zap.String("operation", operation),
				zap.Error(ctx.Err()),
				zap.Int("attempt", attempt+1),
			)
			return errors.New("operation cancelled: " + ctx.Err().Error())
		}
		
		// Calculate backoff with jitter
		if attempt == 0 {
			nextBackoff = retryConfig.InitialBackoff
		} else {
			// Use exponential backoff with jitter
			backoff := float64(retryConfig.InitialBackoff) * math.Pow(2, float64(attempt))
			jitter := rand.Float64() * 0.5 * backoff // Add up to 50% jitter
			nextBackoff = time.Duration(backoff + jitter)
			
			// Cap at max backoff
			if nextBackoff > retryConfig.MaxBackoff {
				nextBackoff = retryConfig.MaxBackoff
			}
		}
		
		logger.Info("Retrying operation after error",
			zap.String("operation", operation),
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("maxAttempts", retryConfig.MaxAttempts),
			zap.Duration("backoff", nextBackoff),
		)
		
		// Sleep with context awareness
		timer := time.NewTimer(nextBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Warn("Context cancelled during backoff",
				zap.String("operation", operation),
				zap.Error(ctx.Err()),
				zap.Int("attempt", attempt+1),
			)
			return errors.New("operation cancelled: " + ctx.Err().Error())
		case <-timer.C:
			// Proceed to next attempt
		}
	}
	
	logger.Error("Maximum retry attempts reached",
		zap.String("operation", operation),
		zap.Error(err),
		zap.Int("maxAttempts", retryConfig.MaxAttempts),
	)
	
	return err
}

// DefaultIsRetryable provides a simple implementation that considers
// network and temporary errors as retryable
func DefaultIsRetryable(err error) bool {
	// In a real implementation, we would check for network errors,
	// rate limits, temporary server errors, etc.
	// For simplicity, this is a placeholder
	return true
} 