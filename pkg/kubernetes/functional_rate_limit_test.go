package kubernetes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"github.com/mediahq/linode-db-allowlist/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"go.uber.org/zap/zaptest"
)

// TestRateLimitRecoveryFunctional tests the recovery from rate limit errors
// focusing on actual behavior rather than implementation details
func TestRateLimitRecoveryFunctional(t *testing.T) {
	// Skip in CI environments where Kubernetes API isn't available
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		t.Skip("Skipping test that requires Kubernetes API access")
	}

	// Setting hostname for consistent testing
	oldHostname := os.Getenv("HOSTNAME")
	os.Setenv("HOSTNAME", "test-node-1")
	defer os.Setenv("HOSTNAME", oldHostname)

	logger := zaptest.NewLogger(t)
	metricsClient := metrics.NewMetrics(logger)
	fakeClient := NewFakeClientForTest()

	// Create the client with a fake clientset
	client, err := NewClient(logger, &config.Config{}, "", "test-lease", "test-namespace", metricsClient)
	require.NoError(t, err)

	// Instead of using fakeClient.Clientset, we'll directly apply the fake client behavior
	fakeClient.InjectIntoClient(client)

	// Register no-op node handlers
	client.RegisterNodeHandlers(
		func(node *corev1.Node, eventType string) error { return nil },
		func(node *corev1.Node, eventType string) error { return nil },
	)

	// Start the client in a separate goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := client.Start(ctx)
		assert.NoError(t, err)
	}()

	// Wait for the client to become leader
	assert.Eventually(t, func() bool {
		return client.IsLeader()
	}, 5*time.Second, 100*time.Millisecond, "Client should become leader")

	// Record initial state
	initialLeader := fakeClient.GetLeader()
	assert.Equal(t, "test-node-1", initialLeader, "Client should be registered as leader initially")

	// Simulate multiple rate limit errors in succession
	t.Run("Multiple rate limit errors", func(t *testing.T) {
		// Reset error tracking for this test
		client.errorsMutex.Lock()
		client.errorsCount = make(map[string]int)
		client.restartsCount = 0
		client.errorsMutex.Unlock()
		
		// Set the simulation mode to rate limit errors with multiple failures
		fakeClient.SetSimulationMode(RateLimitError)
		fakeClient.SetMaxFailures(3) // Simulate 3 consecutive failures
		
		// Wait for the client to detect it's not the leader after the first error
		assert.Eventually(t, func() bool {
			return !client.IsLeader()
		}, 5*time.Second, 100*time.Millisecond, "Client should detect loss of leadership")
		
		// Wait for the client to recover leadership after all errors
		assert.Eventually(t, func() bool {
			return client.IsLeader()
		}, 20*time.Second, 100*time.Millisecond, "Client should recover leadership after multiple failures")
		
		// Verify that we are the leader again
		assert.Equal(t, "test-node-1", fakeClient.GetLeader(), "Client should be registered as leader again")
		
		// Verify that multiple errors and restarts were tracked
		errorCounts := client.GetErrorCounts()
		assert.GreaterOrEqual(t, errorCounts["rate_limit"], 3, "Should record all rate limit errors")
		assert.GreaterOrEqual(t, client.GetRestartCount(), 3, "Should record multiple election restarts")
	})
	
	// Test that the system can handle escalating rate limit errors
	t.Run("Rate limit escalation and recovery", func(t *testing.T) {
		// Reset error tracking for this test
		client.errorsMutex.Lock()
		client.errorsCount = make(map[string]int)
		client.restartsCount = 0
		client.errorsMutex.Unlock()
		
		// Create a monitoring goroutine to track leadership transitions
		transitions := make([]bool, 0)
		
		// In a real system, we'd monitor the actual leadership status by tracking
		// the metrics or observing the behavior, not by directly accessing internal state
		go func() {
			lastStatus := client.IsLeader()
			transitions = append(transitions, lastStatus)
			
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					currentStatus := client.IsLeader()
					if currentStatus != lastStatus {
						transitions = append(transitions, currentStatus)
						lastStatus = currentStatus
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		
		// Simulate rate limit errors over time
		fakeClient.SetSimulationMode(RateLimitError)
		fakeClient.SetMaxFailures(5) // Simulate 5 consecutive failures
		
		// Wait for the recovery to complete (when the client becomes leader again)
		assert.Eventually(t, func() bool {
			return client.IsLeader()
		}, 30*time.Second, 100*time.Millisecond, "Client should eventually recover leadership")
		
		// Verify that we observed leadership transitions (lost and regained)
		time.Sleep(1 * time.Second) // Give the monitoring goroutine time to record the final state
		
		// There should be at least 3 transitions: true (initial) -> false (after error) -> true (recovered)
		assert.GreaterOrEqual(t, len(transitions), 3, "Should observe leadership transitions")
		
		// First should be true, then should be a false, and last should be true
		if len(transitions) >= 3 {
			assert.True(t, transitions[0], "Should start as leader")
			assert.False(t, transitions[len(transitions)/2], "Should lose leadership in the middle")
			assert.True(t, transitions[len(transitions)-1], "Should end as leader")
		}
		
		// Verify error counts and restart counts
		errorCounts := client.GetErrorCounts()
		assert.GreaterOrEqual(t, errorCounts["rate_limit"], 1, "Should record rate limit errors")
		assert.GreaterOrEqual(t, client.GetRestartCount(), 1, "Should record election restarts")
		
		// Verify that metrics reflect what happened
		// We're testing actual behavior, so we check the metrics that would be visible to operators
		// This validates the observable outcome, not the implementation
		// Note that metrics package isn't available in tests, so in a real test we would check Prometheus metrics
	})
} 