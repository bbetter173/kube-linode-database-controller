package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfig tests that default configuration values are set correctly
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default values
	assert.Equal(t, 5, cfg.NodeDeletionDelay)
	assert.Equal(t, 100, cfg.APIRateLimit)
	assert.Equal(t, 5, cfg.Retry.MaxAttempts)
	assert.Equal(t, 1*time.Second, cfg.Retry.InitialBackoff)
	assert.Equal(t, 30*time.Second, cfg.Retry.MaxBackoff)
	assert.Empty(t, cfg.Nodepools)
	assert.Empty(t, cfg.LinodeToken)
}

// TestValidate tests the configuration validation logic
func TestValidate(t *testing.T) {
	// Test cases for validation
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid configuration",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: false,
		},
		{
			name: "Missing Linode token",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "Linode token is required",
		},
		{
			name: "No nodepools",
			config: &Config{
				Nodepools:         []Nodepool{},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "at least one nodepool must be configured",
		},
		{
			name: "Invalid node deletion delay",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: -5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "node deletion delay must be a positive integer",
		},
		{
			name: "Invalid API rate limit",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      0,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "API rate limit must be a positive integer",
		},
		{
			name: "Invalid retry max attempts",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    0,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "retry max attempts must be a positive integer",
		},
		{
			name: "Invalid initial backoff",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: -1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "retry initial backoff must be non-negative",
		},
		{
			name: "Maximum backoff less than initial backoff",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 10 * time.Second,
					MaxBackoff:     5 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "retry max backoff must be greater than or equal to initial backoff",
		},
		{
			name: "Nodepool without name",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "nodepool name cannot be empty",
		},
		{
			name: "Nodepool without databases",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name:      "production",
						Databases: []Database{},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "nodepool production must have at least one database",
		},
		{
			name: "Database without ID",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "database ID cannot be empty in nodepool production",
		},
		{
			name: "Database without name",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: ""},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Second,
					MaxBackoff:     30 * time.Second,
				},
			},
			expectError: true,
			errorMsg:    "database name cannot be empty in nodepool production",
		},
		{
			name: "Zero values for backoff are allowed",
			config: &Config{
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "db1", Name: "prod-db"},
						},
					},
				},
				NodeDeletionDelay: 5,
				APIRateLimit:      100,
				LinodeToken:       "test-token",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 0,
					MaxBackoff:     0,
				},
			},
			expectError: false,
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test loading configuration directly from environment variables
func TestLoadFromEnv(t *testing.T) {
	// Set up environment variables
	os.Setenv("LINODE_TOKEN", "test-token")
	os.Setenv("NODE_DELETION_DELAY", "10")
	os.Setenv("API_RATE_LIMIT", "200")
	
	// Cleanup
	defer func() {
		os.Unsetenv("LINODE_TOKEN")
		os.Unsetenv("NODE_DELETION_DELAY")
		os.Unsetenv("API_RATE_LIMIT")
	}()

	// Load from environment
	cfg := LoadFromEnv()

	// Verify environment values are used correctly
	assert.Equal(t, "test-token", cfg.LinodeToken)
	assert.Equal(t, 10, cfg.NodeDeletionDelay)
	assert.Equal(t, 200, cfg.APIRateLimit)
	
	// Verify other values fall back to defaults
	defaultCfg := DefaultConfig()
	assert.Equal(t, defaultCfg.Retry.MaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, defaultCfg.Retry.InitialBackoff, cfg.Retry.InitialBackoff)
	assert.Equal(t, defaultCfg.Retry.MaxBackoff, cfg.Retry.MaxBackoff)
	assert.Equal(t, len(defaultCfg.Nodepools), len(cfg.Nodepools))
}

// Test environment variables with invalid values
func TestLoadFromEnvWithInvalidValues(t *testing.T) {
	// Set up environment variables with invalid values
	os.Setenv("NODE_DELETION_DELAY", "invalid")
	os.Setenv("API_RATE_LIMIT", "not-a-number")
	os.Setenv("RETRY_MAX_ATTEMPTS", "bad")
	os.Setenv("RETRY_INITIAL_BACKOFF", "not-duration")
	os.Setenv("RETRY_MAX_BACKOFF", "invalid-time")

	// Cleanup
	defer func() {
		os.Unsetenv("NODE_DELETION_DELAY")
		os.Unsetenv("API_RATE_LIMIT")
		os.Unsetenv("RETRY_MAX_ATTEMPTS")
		os.Unsetenv("RETRY_INITIAL_BACKOFF")
		os.Unsetenv("RETRY_MAX_BACKOFF")
	}()

	// Load from environment - should use defaults for invalid values
	cfg := LoadFromEnv()

	// Verify default values are used
	defaultCfg := DefaultConfig()
	assert.Equal(t, defaultCfg.NodeDeletionDelay, cfg.NodeDeletionDelay)
	assert.Equal(t, defaultCfg.APIRateLimit, cfg.APIRateLimit)
	assert.Equal(t, defaultCfg.Retry.MaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, defaultCfg.Retry.InitialBackoff, cfg.Retry.InitialBackoff)
	assert.Equal(t, defaultCfg.Retry.MaxBackoff, cfg.Retry.MaxBackoff)
}

// TestLoadFromFile tests the essential functionality of LoadFromFile
func TestLoadFromFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test with a valid configuration file
	t.Run("Valid config file", func(t *testing.T) {
		validConfig := `
nodepools:
  - name: "production"
    databases:
      - id: "db1"
        name: "prod-db"
nodeDeletionDelay: 10
apiRateLimit: 200
retry:
  maxAttempts: 5
  initialBackoff: 2s
  maxBackoff: 60s
`
		configPath := filepath.Join(tempDir, "valid-config.yaml")
		err := os.WriteFile(configPath, []byte(validConfig), 0644)
		require.NoError(t, err)

		// Load from file
		cfg, err := LoadFromFile(configPath)
		require.NoError(t, err)

		// Verify expected values
		assert.Equal(t, 10, cfg.NodeDeletionDelay)
		assert.Equal(t, 200, cfg.APIRateLimit)
		assert.Equal(t, 5, cfg.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, cfg.Retry.InitialBackoff)
		assert.Equal(t, 60*time.Second, cfg.Retry.MaxBackoff)
		assert.Len(t, cfg.Nodepools, 1)
		assert.Equal(t, "production", cfg.Nodepools[0].Name)
	})

	// Test basic error handling
	t.Run("File errors", func(t *testing.T) {
		// Non-existent file
		cfg, err := LoadFromFile(filepath.Join(tempDir, "nonexistent.yaml"))
		assert.Error(t, err)
		assert.Nil(t, cfg)

		// Invalid YAML
		invalidConfig := "invalid: yaml: ["
		configPath := filepath.Join(tempDir, "invalid-config.yaml")
		err = os.WriteFile(configPath, []byte(invalidConfig), 0644)
		require.NoError(t, err)
		
		cfg, err = LoadFromFile(configPath)
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})
}

// TestLoad tests the combined Load function that handles both file and environment
func TestLoad(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set environment variables for all test cases
	os.Setenv("LINODE_TOKEN", "default-token")
	defer os.Unsetenv("LINODE_TOKEN")

	// Create a valid config file
	validConfig := `
nodepools:
  - name: "production"
    databases:
      - id: "db1"
        name: "prod-db"
nodeDeletionDelay: 10
apiRateLimit: 200
retry:
  maxAttempts: 5
  initialBackoff: 2s
  maxBackoff: 60s
`
	configPath := filepath.Join(tempDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(validConfig), 0644)
	require.NoError(t, err)

	// Test 1: Load from file with minimal environment
	t.Run("Load from file with environment token", func(t *testing.T) {
		// Only token in environment, rest from file
		cfg, err := Load(configPath)
		require.NoError(t, err)

		// Verify correct merging of values
		assert.Equal(t, "default-token", cfg.LinodeToken)
		assert.Equal(t, 10, cfg.NodeDeletionDelay)
		assert.Equal(t, 200, cfg.APIRateLimit)
		assert.Equal(t, 5, cfg.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, cfg.Retry.InitialBackoff)
		assert.Equal(t, 60*time.Second, cfg.Retry.MaxBackoff)
		assert.Len(t, cfg.Nodepools, 1)
		assert.Equal(t, "production", cfg.Nodepools[0].Name)
	})

	// Test 2: Environment variables override file values
	t.Run("Environment overrides file", func(t *testing.T) {
		// Set up environment variables to override file values
		os.Setenv("NODE_DELETION_DELAY", "20")
		os.Setenv("API_RATE_LIMIT", "300")
		defer func() {
			os.Unsetenv("NODE_DELETION_DELAY")
			os.Unsetenv("API_RATE_LIMIT")
		}()

		// Load with all environment overrides
		cfg, err := Load(configPath)
		require.NoError(t, err)

		// Verify env values override file
		assert.Equal(t, "default-token", cfg.LinodeToken)
		assert.Equal(t, 20, cfg.NodeDeletionDelay)
		assert.Equal(t, 300, cfg.APIRateLimit)
		// File values for other fields should remain
		assert.Equal(t, 5, cfg.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, cfg.Retry.InitialBackoff)
		assert.Equal(t, 60*time.Second, cfg.Retry.MaxBackoff)
	})
}

// TestPartialConfig tests loading a configuration with missing fields
func TestPartialConfig(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a partial config file
	partialConfig := `
nodepools:
  - name: "production"
    databases:
      - id: "db1"
        name: "prod-db"
`
	configPath := filepath.Join(tempDir, "partial-config.yaml")
	err = os.WriteFile(configPath, []byte(partialConfig), 0644)
	require.NoError(t, err)

	// Test loading partial config
	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)

	// Verify defaults are used for missing fields
	defaultCfg := DefaultConfig()
	assert.Equal(t, defaultCfg.NodeDeletionDelay, cfg.NodeDeletionDelay)
	assert.Equal(t, defaultCfg.APIRateLimit, cfg.APIRateLimit)
	assert.Equal(t, defaultCfg.Retry.MaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, defaultCfg.Retry.InitialBackoff, cfg.Retry.InitialBackoff)
	assert.Equal(t, defaultCfg.Retry.MaxBackoff, cfg.Retry.MaxBackoff)
	
	// But the provided nodepool config should be present
	assert.Len(t, cfg.Nodepools, 1)
	assert.Equal(t, "production", cfg.Nodepools[0].Name)
	assert.Len(t, cfg.Nodepools[0].Databases, 1)
	assert.Equal(t, "db1", cfg.Nodepools[0].Databases[0].ID)
} 