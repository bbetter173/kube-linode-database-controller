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
	assert.Equal(t, "lke.linode.com/pool-id", cfg.NodepoolLabelKey)
	assert.Equal(t, true, cfg.EnableIPv4)
	assert.Equal(t, false, cfg.EnableIPv6)
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:  time.Second,
					MaxBackoff:      30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Missing Linode token",
			config: Config{
				LinodeToken:  "",
				APIRateLimit: 100,
				LogLevel:     "info",
				EnableIPv4:   true,
				EnableIPv6:   false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				LinodeToken:  "test-token",
				APIRateLimit: 100,
				LogLevel:     "info",
				EnableIPv4:   true,
				EnableIPv6:   false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{},
			},
			expectError: true,
			errorMsg:    "at least one nodepool must be configured",
		},
		{
			name: "Invalid API rate limit",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     0,
				LogLevel:         "info",
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    0,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff: -1 * time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff: 10 * time.Second,
					MaxBackoff:     5 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "", Name: "test-db"},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: ""},
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
				NodepoolLabelKey: "lke.linode.com/pool-id",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff: 0,
					MaxBackoff:     0,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Empty nodepool label key",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				NodepoolLabelKey: "",
				EnableIPv4:       true,
				EnableIPv6:       false,
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "production",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "nodepool label key cannot be empty",
		},
		{
			name: "IP version validation",
			config: Config{
				LinodeToken:      "test-token",
				APIRateLimit:     100,
				LogLevel:         "info",
				NodepoolLabelKey: "lke.linode.com/pool-id",
				Retry: RetryConfig{
					MaxAttempts:    5,
					InitialBackoff:   time.Second,
					MaxBackoff:       30 * time.Second,
				},
				Nodepools: []Nodepool{
					{
						Name: "test-nodepool",
						Databases: []Database{
							{ID: "123", Name: "test-db"},
						},
					},
				},
				EnableIPv4:   false,
				EnableIPv6:   false,
			},
			expectError: true,
			errorMsg:    "at least one IP version (IPv4 or IPv6) must be enabled",
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
	// Save and restore environment
	oldToken := os.Getenv("LINODE_TOKEN")
	oldRateLimit := os.Getenv("API_RATE_LIMIT")
	oldLogLevel := os.Getenv("LOG_LEVEL")
	oldNodepoolLabelKey := os.Getenv("NODEPOOL_LABEL_KEY")
	oldEnableIPv4 := os.Getenv("ENABLE_IPV4")
	oldEnableIPv6 := os.Getenv("ENABLE_IPV6")
	
	defer func() {
		os.Setenv("LINODE_TOKEN", oldToken)
		os.Setenv("API_RATE_LIMIT", oldRateLimit)
		os.Setenv("LOG_LEVEL", oldLogLevel)
		os.Setenv("NODEPOOL_LABEL_KEY", oldNodepoolLabelKey)
		os.Setenv("ENABLE_IPV4", oldEnableIPv4)
		os.Setenv("ENABLE_IPV6", oldEnableIPv6)
	}()
	
	// Set environment variables
	os.Setenv("LINODE_TOKEN", "env-token")
	os.Setenv("API_RATE_LIMIT", "200")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("NODEPOOL_LABEL_KEY", "custom.key/nodepool")
	os.Setenv("ENABLE_IPV4", "false")
	os.Setenv("ENABLE_IPV6", "true")
	
	// Load config from environment
	cfg := LoadFromEnv()
	
	// Verify settings
	assert.Equal(t, "env-token", cfg.LinodeToken)
	assert.Equal(t, 200, cfg.APIRateLimit)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "custom.key/nodepool", cfg.NodepoolLabelKey)
	assert.Equal(t, false, cfg.EnableIPv4)
	assert.Equal(t, true, cfg.EnableIPv6)
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