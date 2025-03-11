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
		
		// Setup mock expectations
		api.On("GetDatabaseAllowList", mock.Anything, "test-db").Return([]string{"1.1.1.1"}, nil).Times(3)
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
		NodeDeletionDelay: 1, // No longer used, node deletion is immediate now
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