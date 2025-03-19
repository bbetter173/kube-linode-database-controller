package linode

import (
	"context"
	"testing"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockLinodeAPI implements the LinodeAPI interface for testing
type MockLinodeAPI struct {
	mock.Mock
}

func (m *MockLinodeAPI) GetDatabaseAllowList(ctx context.Context, dbID string) ([]string, error) {
	args := m.Called(ctx, dbID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockLinodeAPI) UpdateDatabaseAllowList(ctx context.Context, dbID string, allowList []string) error {
	args := m.Called(ctx, dbID, allowList)
	return args.Error(0)
}

func (m *MockLinodeAPI) DeleteAllowListEntry(dbID, ip string) error {
	args := m.Called(dbID, ip)
	return args.Error(0)
}

// MockMetricsClient implements the MetricsClient interface for testing
type MockMetricsClient struct {
	mock.Mock
}

func (m *MockMetricsClient) IncrementAllowListUpdates(database, operation string) {
	m.Called(database, operation)
}

func (m *MockMetricsClient) ObserveAllowListUpdateLatency(database, operation string, latencySeconds float64) {
	m.Called(database, operation, latencySeconds)
}

func (m *MockMetricsClient) UpdatePendingDeletions(nodepool string, count int) {
	m.Called(nodepool, count)
}

func (m *MockMetricsClient) UpdateAPIRateLimitRemaining(api string, remaining float64) {
	m.Called(api, remaining)
}

func (m *MockMetricsClient) IncrementAllowListOpenAccessAlerts(database string) {
	m.Called(database)
}

// TestNewClient tests the NewClient function
func TestNewClient(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Second,
			MaxBackoff: 5 * time.Second,
		},
	}
	metrics := &MockMetricsClient{}
	
	// Create client
	client := NewClient(logger, cfg, metrics)
	
	// Verify client was properly initialized
	assert.NotNil(t, client)
	assert.NotNil(t, client.api)
	assert.NotNil(t, client.rateLimiter)
	assert.Equal(t, logger, client.logger)
	assert.Equal(t, cfg, client.config)
	assert.Equal(t, metrics, client.metrics)
}

// TestUpdateAllowList tests the UpdateAllowList function
func TestUpdateAllowList(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond, // Fast for tests
			MaxBackoff: 5 * time.Millisecond,     // Fast for tests
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
	}
	
	t.Run("Add IP to allow list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Once()
		api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"1.1.1.1", "2.2.2.2"}).Return(nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Once()
		metrics.On("IncrementAllowListUpdates", "test-db-name", "add").Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Skip adding IP already in list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1", "2.2.2.2"}, nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Once()
		// Note: No UpdateDatabaseAllowList or IncrementAllowListUpdates calls expected
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Remove IP from allow list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1", "2.2.2.2"}, nil).Once()
		api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"1.1.1.1"}).Return(nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.AnythingOfType("float64")).Once()
		metrics.On("IncrementAllowListUpdates", "test-db-name", "remove").Once()
		metrics.On("UpdatePendingDeletions", "production", 0).Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "remove")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Skips removal when IP not in list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations - IP is not in the list, so no update should happen
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.AnythingOfType("float64")).Once()
		metrics.On("UpdatePendingDeletions", "production", 0).Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "remove")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
		// Verify that UpdateDatabaseAllowList is not called
		api.AssertNotCalled(t, "UpdateDatabaseAllowList")
	})
	
	t.Run("Detects 0.0.0.0/0 in allow list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1", "0.0.0.0/0"}, nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Once()
		metrics.On("IncrementAllowListOpenAccessAlerts", "test-db-name").Once()
		api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"0.0.0.0/0", "1.1.1.1", "2.2.2.2"}).Return(nil).Once()
		metrics.On("IncrementAllowListUpdates", "test-db-name", "add").Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Handles error from GetDatabaseAllowList", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{}, assert.AnError).Times(3)
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.Error(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Handles error from UpdateDatabaseAllowList", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations - the first GetDatabaseAllowList is used to populate the cache
		// Subsequent calls use the cached value
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Once()
		api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"1.1.1.1", "2.2.2.2"}).Return(assert.AnError).Times(3)
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Once()
		// Note: No IncrementAllowListUpdates call expected because the update failed
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.Error(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Handles unknown nodepool", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function with unknown nodepool
		err := client.UpdateAllowList(context.Background(), "unknown", "test-node", "2.2.2.2", "add")
		
		// Verify results
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no databases configured for nodepool unknown")
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
}

// TestHandleDeleteOperation tests the handleDeleteOperation function
func TestHandleDeleteOperation(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
	}
	
	t.Run("Immediately removes IP from allow list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1", "2.2.2.2"}, nil).Once()
		api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"1.1.1.1"}).Return(nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.AnythingOfType("float64")).Once()
		metrics.On("IncrementAllowListUpdates", "test-db-name", "remove").Once()
		metrics.On("UpdatePendingDeletions", "production", 0).Once() // Called with 0 for immediate deletion
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "remove")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
	})
	
	t.Run("Skips removal when IP not in list", func(t *testing.T) {
		// Create mocks
		api := &MockLinodeAPI{}
		metrics := &MockMetricsClient{}
		
		// Setup mock expectations - IP is not in the list, so no update should happen
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Once()
		metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.AnythingOfType("float64")).Once()
		metrics.On("UpdatePendingDeletions", "production", 0).Once()
		
		// Create client with mocks
		client := NewClientWithAPI(logger, cfg, api, metrics)
		
		// Call the function
		err := client.UpdateAllowList(context.Background(), "production", "test-node", "2.2.2.2", "remove")
		
		// Verify results
		require.NoError(t, err)
		api.AssertExpectations(t)
		metrics.AssertExpectations(t)
		// Verify that UpdateDatabaseAllowList is not called
		api.AssertNotCalled(t, "UpdateDatabaseAllowList")
	})
}

// TestRateLimiter tests the rate limiter functionality
func TestRateLimiter(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 60, // 1 per second
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
	}
	
	// Create mocks
	api := &MockLinodeAPI{}
	metrics := &MockMetricsClient{}
	
	// Setup mock expectations for multiple calls
	api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Times(3)
	api.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"1.1.1.1", "2.2.2.2"}).Return(nil).Times(3)
	metrics.On("ObserveAllowListUpdateLatency", "test-db-name", "add", mock.AnythingOfType("float64")).Times(3)
	metrics.On("IncrementAllowListUpdates", "test-db-name", "add").Times(3)
	
	// Create client with mocks
	client := NewClientWithAPI(logger, cfg, api, metrics)
	
	// Call the rate-limited function multiple times and measure time
	ctx := context.Background()
	startTime := time.Now()
	
	// 3 calls at 1 per second should take about 2 seconds
	for i := 0; i < 3; i++ {
		err := client.UpdateAllowList(ctx, "production", "test-node", "2.2.2.2", "add")
		require.NoError(t, err)
	}
	
	duration := time.Since(startTime)
	
	// Verify rate limiting occurred
	// Should take at least 2 seconds for 3 calls at 1 per second
	// We give some buffer since the first call might be immediate
	assert.True(t, duration >= 1900*time.Millisecond, "Rate limiting should enforce delays between calls")
	
	// Verify all expectations
	api.AssertExpectations(t)
	metrics.AssertExpectations(t)
}

// TestNormalizeIP tests the normalizeIP function
func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IP without CIDR",
			input:    "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "IP with /32 CIDR",
			input:    "192.168.1.1/32",
			expected: "192.168.1.1",
		},
		{
			name:     "IP with /24 CIDR",
			input:    "192.168.1.0/24",
			expected: "192.168.1.0/24", // Should not be stripped, as it's an actual subnet
		},
		{
			name:     "IPv6 with /128 CIDR",
			input:    "2001:db8::1/128",
			expected: "2001:db8::1",
		},
		{
			name:     "IPv6 with /64 CIDR",
			input:    "2001:db8::/64",
			expected: "2001:db8::/64", // Should not be stripped, as it's an actual subnet
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeIP(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRemoveIPWithCIDR tests removing an IP when the allow list contains CIDR notation
func TestRemoveIPWithCIDR(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
	}

	// Create mocks
	mockAPI := &MockLinodeAPI{}
	mockMetrics := &MockMetricsClient{}

	// Set up expectations for the test
	// The allow list contains the IP with CIDR notation
	mockAPI.On("GetDatabaseAllowList", mock.Anything, "test-db").Return(
		[]string{"172.105.162.60/32", "192.168.1.1"}, nil,
	)
	
	// Expect the API to be called with the updated allow list (without the IP)
	mockAPI.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"192.168.1.1"}).Return(nil)
	
	// Metrics expectations
	mockMetrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.Anything).Return()
	mockMetrics.On("IncrementAllowListUpdates", "test-db-name", "remove").Return()
	mockMetrics.On("UpdatePendingDeletions", "production", 0).Return()

	// Create client with mocks
	client := NewClientWithAPI(logger, cfg, mockAPI, mockMetrics)

	// Test removing an IP that exists in the allow list with CIDR notation
	// We're trying to remove 172.105.162.60 but it's stored as 172.105.162.60/32
	err := client.handleDeleteOperation(context.Background(), "production", cfg.Nodepools[0].Databases, "test-node", "172.105.162.60")
	
	// Verify expectations
	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestDoesNotRemoveSubnet verifies that an IP with subnet mask (non-/32 CIDR) won't be removed
// when trying to remove an IP that matches the subnet's prefix
func TestDoesNotRemoveSubnet(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
	}

	// Create mocks
	mockAPI := &MockLinodeAPI{}
	mockMetrics := &MockMetricsClient{}

	// Set up expectations for the test
	// The allow list contains a subnet (non-/32 CIDR)
	mockAPI.On("GetDatabaseAllowList", mock.Anything, "test-db").Return(
		[]string{"192.168.1.0/24", "10.0.0.1"}, nil,
	)
	
	// No changes should be made to the allow list since 192.168.1.1 should not match 192.168.1.0/24
	// We should receive the full list back
	mockAPI.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"192.168.1.0/24", "10.0.0.1"}).Return(nil).Maybe()
	
	// Metrics expectations
	mockMetrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.Anything).Return()
	mockMetrics.On("IncrementAllowListUpdates", "test-db-name", "remove").Return().Maybe()
	mockMetrics.On("UpdatePendingDeletions", "production", 0).Return()

	// Create client with mocks
	client := NewClientWithAPI(logger, cfg, mockAPI, mockMetrics)

	// Call the function under test - try to remove 192.168.1.1 when 192.168.1.0/24 is in the list
	err := client.UpdateAllowList(context.Background(), "production", "test-node", "192.168.1.1", "remove")
	
	// Verify expectations
	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestRemoveIPv6WithCIDR tests removing an IPv6 address when the allow list contains the IPv6 with /128 CIDR notation
func TestRemoveIPv6WithCIDR(t *testing.T) {
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		LinodeToken: "test-token",
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		},
		Nodepools: []config.Nodepool{
			{
				Name: "production",
				Databases: []config.Database{
					{ID: "test-db", Name: "test-db-name"},
				},
			},
		},
		EnableIPv6: true,
	}

	// Create mocks
	mockAPI := &MockLinodeAPI{}
	mockMetrics := &MockMetricsClient{}

	// Set up expectations for the test
	// The allow list contains IPv6 addresses, one with /128 CIDR notation
	mockAPI.On("GetDatabaseAllowList", mock.Anything, "test-db").Return(
		[]string{"2001:db8::1/128", "2001:db8::2"}, nil,
	)
	
	// Expect the API to be called with the updated allow list (without the IPv6 address)
	mockAPI.On("UpdateDatabaseAllowList", mock.Anything, "test-db", []string{"2001:db8::2"}).Return(nil)
	
	// Metrics expectations
	mockMetrics.On("ObserveAllowListUpdateLatency", "test-db-name", "remove", mock.Anything).Return()
	mockMetrics.On("IncrementAllowListUpdates", "test-db-name", "remove").Return()
	mockMetrics.On("UpdatePendingDeletions", "production", 0).Return()

	// Create client with mocks
	client := NewClientWithAPI(logger, cfg, mockAPI, mockMetrics)

	// Test removing an IPv6 address that exists in the allow list with CIDR notation
	// We're trying to remove 2001:db8::1 but it's stored as 2001:db8::1/128
	err := client.handleDeleteOperation(context.Background(), "production", cfg.Nodepools[0].Databases, "test-node", "2001:db8::1")
	
	// Verify expectations
	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestCacheOperations tests the allow list caching functionality
func TestCacheOperations(t *testing.T) {
	// Create mock API
	mockAPI := &MockLinodeAPI{}
	
	// Create test dependencies
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{
		APIRateLimit: 100,
		Retry: config.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: 1 * time.Millisecond, // Fast for tests
			MaxBackoff:     5 * time.Millisecond, // Fast for tests
		},
		Nodepools: []config.Nodepool{
			{
				Name: "test-pool",
				Databases: []config.Database{
					{
						ID:   "1",
						Name: "test-db",
					},
				},
			},
		},
	}
	
	mockMetrics := &MockMetricsClient{}
	mockMetrics.On("ObserveAllowListUpdateLatency", mock.Anything, mock.Anything, mock.Anything).Return()
	mockMetrics.On("IncrementAllowListUpdates", mock.Anything, mock.Anything).Return()
	mockMetrics.On("IncrementAllowListOpenAccessAlerts", mock.Anything).Return().Maybe()
	
	// Setup expected API calls for initial cache load
	mockAPI.On("GetDatabaseAllowList", mock.Anything, "1").Return([]string{"192.168.1.1"}, nil).Once()
	
	// Create client with mock API
	client := NewClientWithAPI(logger, cfg, mockAPI, mockMetrics)
	
	// Initialize cache
	ctx := context.Background()
	err := client.InitializeCache(ctx)
	require.NoError(t, err)
	
	// Verify cache was loaded
	allowList, err := client.getCachedAllowList(ctx, "1", "test-db")
	require.NoError(t, err)
	assert.Equal(t, []string{"192.168.1.1"}, allowList)
	
	// Set up API expectations for cache invalidation test
	// First call should use cache (no API call)
	// After update, cache will be invalidated, so second call should call API
	mockAPI.On("UpdateDatabaseAllowList", mock.Anything, "1", []string{"192.168.1.1", "192.168.1.2"}).Return(nil).Once()
	mockAPI.On("GetDatabaseAllowList", mock.Anything, "1").Return([]string{"192.168.1.1", "192.168.1.2"}, nil).Once()
	
	// Test updating the allow list (should invalidate cache)
	err = client.updateDatabaseAllowList(ctx, "1", "test-db", "test-node", "192.168.1.2", "add")
	require.NoError(t, err)
	
	// Verify cached list is updated after invalidation
	allowList, err = client.getCachedAllowList(ctx, "1", "test-db")
	require.NoError(t, err)
	assert.Equal(t, []string{"192.168.1.1", "192.168.1.2"}, allowList)
	
	// Verify all expected API calls were made
	mockAPI.AssertExpectations(t)
} 