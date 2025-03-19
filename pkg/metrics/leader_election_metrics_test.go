package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestLeaderElectionMetrics tests the leader election metrics functionality
func TestLeaderElectionMetrics(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Test SetLeaderStatus
	t.Run("TestSetLeaderStatus", func(t *testing.T) {
		// Reset the gauge for our test
		m.LeaderStatus.Set(0)
		
		// Test becoming leader
		m.SetLeaderStatus(true)
		val := testutil.ToFloat64(m.LeaderStatus)
		assert.Equal(t, float64(1), val, "Leader status should be 1 when leader")
		
		// Test losing leadership
		m.SetLeaderStatus(false)
		val = testutil.ToFloat64(m.LeaderStatus)
		assert.Equal(t, float64(0), val, "Leader status should be 0 when not leader")
	})
	
	// Test IncrementLeaderElectionError
	t.Run("TestIncrementLeaderElectionError", func(t *testing.T) {
		// Reset the counter for our test
		m.LeaderElectionErrors.Reset()
		
		// Increment different error types
		m.IncrementLeaderElectionError("rate_limit")
		val := testutil.ToFloat64(m.LeaderElectionErrors.WithLabelValues("rate_limit"))
		assert.Equal(t, float64(1), val, "Error counter should be incremented for rate_limit")
		
		m.IncrementLeaderElectionError("timeout")
		val = testutil.ToFloat64(m.LeaderElectionErrors.WithLabelValues("timeout"))
		assert.Equal(t, float64(1), val, "Error counter should be incremented for timeout")
		
		// Increment the same error type again
		m.IncrementLeaderElectionError("rate_limit")
		val = testutil.ToFloat64(m.LeaderElectionErrors.WithLabelValues("rate_limit"))
		assert.Equal(t, float64(2), val, "Error counter should be incremented twice for rate_limit")
	})
	
	// Test IncrementLeaderElectionRestart
	t.Run("TestIncrementLeaderElectionRestart", func(t *testing.T) {
		// Create a test registry and counter for testing
		registry := prometheus.NewRegistry()
		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter",
		})
		registry.MustRegister(counter)
		
		// Reset the counter for our test by creating a new metrics instance
		testMetrics := NewMetrics(logger)
		
		// Increment restart counter
		testMetrics.IncrementLeaderElectionRestart()
		val := testutil.ToFloat64(testMetrics.LeaderElectionRestarts)
		assert.Equal(t, float64(1), val, "Restart counter should be incremented")
		
		// Increment again
		testMetrics.IncrementLeaderElectionRestart()
		val = testutil.ToFloat64(testMetrics.LeaderElectionRestarts)
		assert.Equal(t, float64(2), val, "Restart counter should be incremented twice")
	})
	
	// Test all leader election metrics together
	t.Run("TestIntegratedLeaderElectionMetrics", func(t *testing.T) {
		// Create a new metrics instance to start with clean counters
		testMetrics := NewMetrics(logger)
		
		// 1. Initial leader state
		testMetrics.SetLeaderStatus(true)
		
		// 2. Error occurs, lose leadership
		testMetrics.SetLeaderStatus(false)
		testMetrics.IncrementLeaderElectionError("rate_limit")
		testMetrics.IncrementLeaderElectionRestart()
		
		// 3. After restart, become leader again
		testMetrics.SetLeaderStatus(true)
		
		// 4. Another error occurs
		testMetrics.SetLeaderStatus(false)
		testMetrics.IncrementLeaderElectionError("timeout")
		testMetrics.IncrementLeaderElectionRestart()
		
		// 5. After second restart, become leader again
		testMetrics.SetLeaderStatus(true)
		
		// Check final metrics state
		assert.Equal(t, float64(1), testutil.ToFloat64(testMetrics.LeaderStatus), 
			"Leader status should be 1 at the end")
		
		assert.Equal(t, float64(1), testutil.ToFloat64(testMetrics.LeaderElectionErrors.WithLabelValues("rate_limit")), 
			"Should have 1 rate_limit error")
		
		assert.Equal(t, float64(1), testutil.ToFloat64(testMetrics.LeaderElectionErrors.WithLabelValues("timeout")),
			"Should have 1 timeout error")
		
		assert.Equal(t, float64(2), testutil.ToFloat64(testMetrics.LeaderElectionRestarts),
			"Should have 2 restart events")
	})
} 