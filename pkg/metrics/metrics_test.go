package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRegisterMetrics(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register the metrics
	m.RegisterMetrics()

	// We can't directly check registration since we're using the default registry
	// Instead, we'll verify that our metrics functions don't panic
	assert.NotPanics(t, func() {
		m.IncrementNodeCount("test")
		m.DecrementNodeCount("test")
		m.SetNodeCount("test", 1)
		m.ObserveOperationDuration("add", "success", 1.5)
	})
}

func TestIncrementNodeCount(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the counter for our test
	m.NodesWatched.Reset()

	// Initial counter should be 0
	val := testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(0), val)

	// Increment the counter
	m.IncrementNodeCount("production")

	// Counter should now be 1
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(1), val)

	// Increment again
	m.IncrementNodeCount("production")

	// Counter should now be 2
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(2), val)

	// Try a different label
	m.IncrementNodeCount("staging")
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("staging"))
	assert.Equal(t, float64(1), val)
}

func TestDecrementNodeCount(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the counter for our test
	m.NodesWatched.Reset()

	// Set initial value to 2
	m.NodesWatched.WithLabelValues("production").Add(2)

	// Decrement the counter
	m.DecrementNodeCount("production")

	// Counter should now be 1
	val := testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(1), val)

	// Decrement again
	m.DecrementNodeCount("production")

	// Counter should now be 0
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(0), val)

	// Decrement below 0 (should stay at 0)
	m.DecrementNodeCount("production")
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(0), val)
}

func TestSetNodeCount(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the counter for our test
	m.NodesWatched.Reset()

	// Set the counter to 5
	m.SetNodeCount("production", 5)

	// Counter should be 5
	val := testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(5), val)

	// Set to a different value
	m.SetNodeCount("production", 10)
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("production"))
	assert.Equal(t, float64(10), val)

	// Try a different label
	m.SetNodeCount("staging", 3)
	val = testutil.ToFloat64(m.NodesWatched.WithLabelValues("staging"))
	assert.Equal(t, float64(3), val)
}

func TestObserveOperationDuration(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Observe a duration for an operation - this should not panic
	assert.NotPanics(t, func() {
		m.ObserveOperationDuration("add", "success", 1.5)
	})
}

func TestObserveAllowListUpdateLatency(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Observe update latency - this should not panic
	assert.NotPanics(t, func() {
		m.ObserveAllowListUpdateLatency("db1", "add", 0.1)
	})
}

func TestIncrementAllowListOpenAccessAlerts(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the counter for our test
	m.AllowListOpenAccessAlerts.Reset()

	// Increment alerts counter
	m.IncrementAllowListOpenAccessAlerts("db1")

	// Counter should now be 1
	val := testutil.ToFloat64(m.AllowListOpenAccessAlerts.WithLabelValues("db1"))
	assert.Equal(t, float64(1), val)

	// Increment again
	m.IncrementAllowListOpenAccessAlerts("db1")
	val = testutil.ToFloat64(m.AllowListOpenAccessAlerts.WithLabelValues("db1"))
	assert.Equal(t, float64(2), val)

	// Try a different label
	m.IncrementAllowListOpenAccessAlerts("db2")
	val = testutil.ToFloat64(m.AllowListOpenAccessAlerts.WithLabelValues("db2"))
	assert.Equal(t, float64(1), val)
}

func TestSetLeaderStatus(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the gauge for our test
	m.LeaderStatus.Set(0)

	// Set as leader
	m.SetLeaderStatus(true)
	val := testutil.ToFloat64(m.LeaderStatus)
	assert.Equal(t, float64(1), val)

	// Set as not leader
	m.SetLeaderStatus(false)
	val = testutil.ToFloat64(m.LeaderStatus)
	assert.Equal(t, float64(0), val)
}

func TestUpdatePendingDeletions(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)
	
	// Reset the gauge for our test
	m.PendingDeletions.Reset()

	// Set pending deletions for a nodepool
	m.UpdatePendingDeletions("production", 5)
	val := testutil.ToFloat64(m.PendingDeletions.WithLabelValues("production"))
	assert.Equal(t, float64(5), val)

	// Update the value
	m.UpdatePendingDeletions("production", 3)
	val = testutil.ToFloat64(m.PendingDeletions.WithLabelValues("production"))
	assert.Equal(t, float64(3), val)

	// Try a different nodepool
	m.UpdatePendingDeletions("staging", 2)
	val = testutil.ToFloat64(m.PendingDeletions.WithLabelValues("staging"))
	assert.Equal(t, float64(2), val)
}

func TestHandler(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register metrics and add some values to ensure they show up
	m.RegisterMetrics()

	// Set values for metrics that will be checked
	m.NodesWatched.WithLabelValues("production").Set(5)
	m.NodeAddsTotal.WithLabelValues("production").Inc()
	m.NodeDeletesTotal.WithLabelValues("staging").Inc()
	m.AllowListOpenAccessAlerts.WithLabelValues("db1").Inc()
	m.PendingDeletions.WithLabelValues("production").Set(2)
	m.LeaderStatus.Set(1)
	
	// Create a test HTTP server using the metrics handler
	handler := promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
	server := httptest.NewServer(handler)
	defer server.Close()
	
	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Check the response
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	
	// The response should contain the expected metrics names
	responseStr := string(body)
	assert.Contains(t, responseStr, "nodes_watched")
	assert.Contains(t, responseStr, "node_adds_total")
	assert.Contains(t, responseStr, "node_deletes_total")
	assert.Contains(t, responseStr, "allow_list_open_access_alerts")
	assert.Contains(t, responseStr, "leader_status")
	assert.Contains(t, responseStr, "pending_deletions")
}

