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
	assert.Equal(t, 100, cfg.APIRateLimit)
	assert.Equal(t, 5, cfg.Retry.MaxAttempts)
	assert.Equal(t, 1*time.Second, cfg.Retry.InitialBackoff)
	assert.Equal(t, 30*time.Second, cfg.Retry.MaxBackoff)
	assert.Empty(t, cfg.Nodepools)
	assert.Empty(t, cfg.LinodeToken)
	assert.Equal(t, "info", cfg.LogLevel)
}

// TestValidate tests the configuration validation logic
func TestValidate(t *testing.T) {
	// Test cases for validation
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid configuration",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 1 * time.Millisecond,
					MaxBackoff:     5 * time.Millisecond,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Missing Linode token",
			config: Config{
				APIRateLimit:     100,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "Linode token is required",
		},
		{
			name: "No nodepools",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
			},
			expectError: true,
			errorMsg:    "at least one nodepool must be configured",
		},
		{
			name: "Invalid API rate limit",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     -5,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "API rate limit must be a positive integer",
		},
		{
			name: "Invalid retry max attempts",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Retry: RetryConfig{
					MaxAttempts:    0,
					InitialBackoff: 1 * time.Millisecond,
					MaxBackoff:     5 * time.Millisecond,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "retry max attempts must be a positive integer",
		},
		{
			name: "Invalid initial backoff",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: -1 * time.Millisecond,
					MaxBackoff:     5 * time.Millisecond,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "retry initial backoff must be non-negative",
		},
		{
			name: "Maximum backoff less than initial backoff",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 10 * time.Millisecond,
					MaxBackoff:     5 * time.Millisecond,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "retry max backoff must be greater than or equal to initial backoff",
		},
		{
			name: "Nodepool without name",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name: "",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nodepool name cannot be empty",
		},
		{
			name: "Nodepool without databases",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name:      "production",
						Databases: []Database{},
					},
				},
			},
			expectError: true,
			errorMsg:    "nodepool production must have at least one database",
		},
		{
			name: "Database without ID",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "",
								Name: "test-db-name",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "database ID cannot be empty in nodepool production",
		},
		{
			name: "Database without name",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "",
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "database name cannot be empty in nodepool production",
		},
		{
			name: "Zero values for backoff are allowed",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				Retry: RetryConfig{
					MaxAttempts:    3,
					InitialBackoff: 0,
					MaxBackoff:     0,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{
								ID:   "test-db",
								Name: "test-db-name",
							},
						},
					},
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
	os.Setenv("API_RATE_LIMIT", "200")
	os.Setenv("LOG_LEVEL", "debug")
	
	// Cleanup
	defer func() {
		os.Unsetenv("LINODE_TOKEN")
		os.Unsetenv("API_RATE_LIMIT")
		os.Unsetenv("LOG_LEVEL")
	}()

	// Load from environment
	cfg := LoadFromEnv()

	// Verify expected values
	assert.Equal(t, "test-token", cfg.LinodeToken)
	assert.Equal(t, 200, cfg.APIRateLimit)
	assert.Equal(t, "debug", cfg.LogLevel)
	
	// Verify other values fall back to defaults
	defaultCfg := DefaultConfig()
	assert.Equal(t, defaultCfg.Retry.MaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, defaultCfg.Retry.InitialBackoff, cfg.Retry.InitialBackoff)
	assert.Equal(t, defaultCfg.Retry.MaxBackoff, cfg.Retry.MaxBackoff)
}

// Test loading environment variables with invalid values
func TestLoadFromEnvWithInvalidValues(t *testing.T) {
	// Setup with invalid values
	os.Setenv("API_RATE_LIMIT", "-10") // Negative, should use default
	defer os.Unsetenv("API_RATE_LIMIT")

	// Load from environment
	cfg := LoadFromEnv()

	// Should use default despite invalid value
	defaultCfg := DefaultConfig()
	assert.Equal(t, defaultCfg.APIRateLimit, cfg.APIRateLimit)
}

// TestLoadFromFile tests loading configuration from a YAML file
func TestLoadFromFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("Valid config file", func(t *testing.T) {
		validConfig := `
nodepools:
  - name: "production"
    databases:
      - id: "db1"
        name: "prod-db"
apiRateLimit: 200
retry:
  maxAttempts: 5
  initialBackoff: 2s
  maxBackoff: 60s
logLevel: "debug"
`
		configPath := filepath.Join(tempDir, "valid-config.yaml")
		err := os.WriteFile(configPath, []byte(validConfig), 0644)
		require.NoError(t, err)

		// Load from file
		cfg, err := LoadFromFile(configPath)
		require.NoError(t, err)

		// Verify expected values
		assert.Equal(t, 200, cfg.APIRateLimit)
		assert.Equal(t, 5, cfg.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, cfg.Retry.InitialBackoff)
		assert.Equal(t, 60*time.Second, cfg.Retry.MaxBackoff)
		assert.Equal(t, "debug", cfg.LogLevel)
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
apiRateLimit: 200
retry:
  maxAttempts: 5
  initialBackoff: 2s
  maxBackoff: 60s
logLevel: "info"
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
		assert.Equal(t, 200, cfg.APIRateLimit)
		assert.Equal(t, 5, cfg.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, cfg.Retry.InitialBackoff)
		assert.Equal(t, 60*time.Second, cfg.Retry.MaxBackoff)
		assert.Equal(t, "info", cfg.LogLevel)
		assert.Len(t, cfg.Nodepools, 1)
		assert.Equal(t, "production", cfg.Nodepools[0].Name)
	})

	// Test 2: Environment variables override file values
	t.Run("Environment overrides file", func(t *testing.T) {
		// Set up environment variables to override file values
		os.Setenv("API_RATE_LIMIT", "300")
		os.Setenv("LOG_LEVEL", "debug")
		defer func() {
			os.Unsetenv("API_RATE_LIMIT")
			os.Unsetenv("LOG_LEVEL")
		}()

		// Load with all environment overrides
		cfg, err := Load(configPath)
		require.NoError(t, err)

		// Verify env values override file
		assert.Equal(t, "default-token", cfg.LinodeToken)
		assert.Equal(t, 300, cfg.APIRateLimit)
		assert.Equal(t, "debug", cfg.LogLevel)
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
	assert.Equal(t, defaultCfg.APIRateLimit, cfg.APIRateLimit)
	assert.Equal(t, defaultCfg.Retry.MaxAttempts, cfg.Retry.MaxAttempts)
	assert.Equal(t, defaultCfg.Retry.InitialBackoff, cfg.Retry.InitialBackoff)
	assert.Equal(t, defaultCfg.Retry.MaxBackoff, cfg.Retry.MaxBackoff)
	assert.Equal(t, defaultCfg.LogLevel, cfg.LogLevel)
	
	// But the provided nodepool config should be present
	assert.Len(t, cfg.Nodepools, 1)
	assert.Equal(t, "production", cfg.Nodepools[0].Name)
	assert.Len(t, cfg.Nodepools[0].Databases, 1)
	assert.Equal(t, "db1", cfg.Nodepools[0].Databases[0].ID)
}