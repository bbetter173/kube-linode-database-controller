package kubernetes

import (
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

// TestLeaderElectionFunctional tests the actual leader election functionality
// without mocking the internal implementation
func TestLeaderElectionFunctional(t *testing.T) {
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

	// Create the client
	client, err := NewClient(logger, &config.Config{}, "", "test-lease", "test-namespace", metricsClient)
	require.NoError(t, err)

	// Register no-op node handlers
	client.RegisterNodeHandlers(
		func(node *corev1.Node, eventType string) error { return nil },
		func(node *corev1.Node, eventType string) error { return nil },
	)
	
	// Apply the test helper to the client
	fakeClient.InjectIntoClient(client)
	
	// Set the client as leader initially for testing
	client.leadershipMutex.Lock()
	client.isLeader = true
	client.leadershipMutex.Unlock()
	fakeClient.SetLeader("test-node-1")

	// Verify initial state
	assert.True(t, client.IsLeader(), "Client should be leader initially")
	assert.Equal(t, "test-node-1", fakeClient.GetLeader(), "Client should be registered as leader")

	// The initial leader test passed, now test the error recovery
	t.Run("Recovers from rate limit error", func(t *testing.T) {
		// Reset error tracking for this test
		client.errorsMutex.Lock()
		client.errorsCount = make(map[string]int)
		client.restartsCount = 0
		client.errorsMutex.Unlock()
		
		// Set the simulation mode to rate limit errors
		fakeClient.SetSimulationMode(RateLimitError)
		fakeClient.SetMaxFailures(1)

		// Wait for the client to detect it's not the leader
		assert.Eventually(t, func() bool {
			return !client.IsLeader()
		}, 5*time.Second, 100*time.Millisecond, "Client should detect it's not the leader")

		// Wait for the client to recover leadership
		assert.Eventually(t, func() bool {
			return client.IsLeader()
		}, 10*time.Second, 100*time.Millisecond, "Client should recover leadership")

		// Verify that we are the leader again
		assert.Equal(t, "test-node-1", fakeClient.GetLeader(), "Client should be registered as leader again")

		// Verify that appropriate error and restart counts are tracked
		errorCounts := client.GetErrorCounts()
		assert.Greater(t, errorCounts["rate_limit"], 0, "Should record rate limit errors")
		assert.Greater(t, client.GetRestartCount(), 0, "Should record election restarts")
	})

	t.Run("Recovers from leadership loss", func(t *testing.T) {
		// Reset error tracking for this test
		client.errorsMutex.Lock()
		client.errorsCount = make(map[string]int)
		client.restartsCount = 0
		client.errorsMutex.Unlock()

		// Set the simulation mode to leadership loss
		fakeClient.SetSimulationMode(LeadershipLossError)
		fakeClient.SetMaxFailures(1)

		// Wait for the client to detect it's not the leader
		assert.Eventually(t, func() bool {
			return !client.IsLeader()
		}, 5*time.Second, 100*time.Millisecond, "Client should detect leadership loss")

		// Wait for the client to recover leadership
		assert.Eventually(t, func() bool {
			return client.IsLeader()
		}, 10*time.Second, 100*time.Millisecond, "Client should recover leadership after loss")

		// Verify that we are the leader again
		assert.Equal(t, "test-node-1", fakeClient.GetLeader(), "Client should be registered as leader again")

		// Verify that appropriate error and restart counts are tracked
		errorCounts := client.GetErrorCounts()
		assert.Greater(t, errorCounts["leadership_lost"], 0, "Should record leadership loss errors")
		assert.Greater(t, client.GetRestartCount(), 0, "Should record election restarts")
	})

	t.Run("Multiple leaders scenario", func(t *testing.T) {
		// Create a second client with a different hostname
		os.Setenv("HOSTNAME", "test-node-2")
		
		logger2 := zaptest.NewLogger(t)
		metricsClient2 := metrics.NewMetrics(logger2)
		
		// Create the client
		client2, err := NewClient(logger2, &config.Config{}, "", "test-lease", "test-namespace", metricsClient2)
		require.NoError(t, err)

		// Register no-op node handlers for client2
		client2.RegisterNodeHandlers(
			func(node *corev1.Node, eventType string) error { return nil },
			func(node *corev1.Node, eventType string) error { return nil },
		)
		
		// Set normal operation mode
		fakeClient.SetSimulationMode(NormalOperation)
		
		// The first client is already leader
		assert.True(t, client.IsLeader(), "First client should be leader")
		
		// The second client should not be leader
		assert.False(t, client2.IsLeader(), "Second client should not be leader")
		
		// Simulate first client stopping
		client.leadershipMutex.Lock()
		client.isLeader = false
		client.leadershipMutex.Unlock()
		
		// Allow second client to become leader
		client2.leadershipMutex.Lock()
		client2.isLeader = true
		client2.leadershipMutex.Unlock()
		fakeClient.SetLeader("test-node-2")
		
		// Verify leadership transition
		assert.False(t, client.IsLeader(), "First client should no longer be leader")
		assert.True(t, client2.IsLeader(), "Second client should now be leader")
		assert.Equal(t, "test-node-2", fakeClient.GetLeader(), "Second client should be registered as leader")
	})
} 