package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestHealthHandler tests the health check endpoint
func TestHealthHandler(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Create a test HTTP server with a custom handler for health
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Make a request to the health endpoint
	resp, err := http.Get(server.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "OK", string(body))
}

// TestMetricsEndpoint tests the metrics endpoint
func TestMetricsEndpoint(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register some metrics
	m.RegisterMetrics()
	
	// Set some values
	m.NodesWatched.WithLabelValues("test").Set(5)
	m.AllowListOpenAccessAlerts.WithLabelValues("test-db").Inc()

	// Create a test HTTP server using the metrics handler
	handler := m.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// The response should contain our metrics
	responseStr := string(body)
	assert.Contains(t, responseStr, "nodes_watched")
	assert.Contains(t, responseStr, "allow_list_open_access_alerts")
}

// TestInvalidEndpoint tests accessing an invalid endpoint
func TestInvalidEndpoint(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Create a test HTTP server with a custom mux that explicitly handles paths
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	// Add no other handlers, so /invalid will be 404
	server := httptest.NewServer(mux)
	defer server.Close()

	// Make a request to an invalid endpoint
	resp, err := http.Get(server.URL + "/invalid")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the response - should be 404
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestConcurrentRequests tests handling concurrent requests
func TestConcurrentRequests(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register metrics
	m.RegisterMetrics()

	// Create a test HTTP server using the metrics handler
	handler := m.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make multiple concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(server.URL + "/metrics")
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}()
	}

	// Wait for all requests to complete
	wg.Wait()
}

// TestContentTypeHeader tests that the metrics endpoint returns the correct content type
func TestContentTypeHeader(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register metrics
	m.RegisterMetrics()

	// Create a test HTTP server using the metrics handler
	handler := m.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the content type header
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "text/plain")
}

// TestHeaderWithContent tests that all headers and content are correctly set
func TestHeaderWithContent(t *testing.T) {
	// Create a metrics client
	logger := zaptest.NewLogger(t)
	m := NewMetrics(logger)

	// Register metrics
	m.RegisterMetrics()

	// Create a test HTTP server using the metrics handler
	handler := m.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a request to the metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the response code
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check the content type
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "text/plain")

	// Check that we got some content
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Greater(t, len(body), 0)
} 