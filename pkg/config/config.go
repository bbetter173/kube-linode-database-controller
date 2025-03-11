package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

// Database represents a Linode Managed Database
type Database struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// Nodepool represents a Kubernetes nodepool and its associated databases
type Nodepool struct {
	Name      string     `yaml:"name"`
	Databases []Database `yaml:"databases"`
}

// RetryConfig holds retry settings for API calls
type RetryConfig struct {
	MaxAttempts    int           `yaml:"maxAttempts"`
	InitialBackoff time.Duration `yaml:"initialBackoff"`
	MaxBackoff     time.Duration `yaml:"maxBackoff"`
}

// Config represents the application configuration
type Config struct {
	Nodepools         []Nodepool  `yaml:"nodepools"`
	NodeDeletionDelay int         `yaml:"nodeDeletionDelay"`
	APIRateLimit      int         `yaml:"apiRateLimit"`
	LinodeToken       string      `yaml:"-"` // Not stored in config file
	Retry             RetryConfig `yaml:"retry"`
}

// DefaultConfig returns a configuration with reasonable defaults
func DefaultConfig() *Config {
	return &Config{
		Nodepools:         []Nodepool{},
		NodeDeletionDelay: 5, // 5 minutes
		APIRateLimit:      100,
		Retry: RetryConfig{
			MaxAttempts:    5,
			InitialBackoff: time.Second,
			MaxBackoff:     30 * time.Second,
		},
	}
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	config := DefaultConfig()

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return config, nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	config := DefaultConfig()

	// Load Linode token from environment
	config.LinodeToken = os.Getenv("LINODE_TOKEN")

	// Override other settings from environment if provided
	if val := os.Getenv("NODE_DELETION_DELAY"); val != "" {
		var delay int
		if _, err := fmt.Sscanf(val, "%d", &delay); err == nil && delay > 0 {
			config.NodeDeletionDelay = delay
		}
	}

	if val := os.Getenv("API_RATE_LIMIT"); val != "" {
		var limit int
		if _, err := fmt.Sscanf(val, "%d", &limit); err == nil && limit > 0 {
			config.APIRateLimit = limit
		}
	}

	return config
}

// Load attempts to load config from a file if it exists, then applies environment variable overrides
func Load(configPath string) (*Config, error) {
	var config *Config
	
	// First try to load from file if specified
	if configPath != "" {
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, fmt.Errorf("invalid config path: %w", err)
		}

		if _, err := os.Stat(absPath); err == nil {
			fileConfig, err := LoadFromFile(absPath)
			if err != nil {
				return nil, err
			}
			config = fileConfig
		} else {
			// If file doesn't exist, start with default config
			config = DefaultConfig()
		}
	} else {
		// No file specified, start with default config
		config = DefaultConfig()
	}

	// Apply environment variable overrides
	// Load Linode token from environment
	if token := os.Getenv("LINODE_TOKEN"); token != "" {
		config.LinodeToken = token
	}

	// Override other settings from environment if provided
	if val := os.Getenv("NODE_DELETION_DELAY"); val != "" {
		var delay int
		if _, err := fmt.Sscanf(val, "%d", &delay); err == nil && delay > 0 {
			config.NodeDeletionDelay = delay
		}
	}

	if val := os.Getenv("API_RATE_LIMIT"); val != "" {
		var limit int
		if _, err := fmt.Sscanf(val, "%d", &limit); err == nil && limit > 0 {
			config.APIRateLimit = limit
		}
	}
	
	// Validate the final configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check for required Linode API token
	if c.LinodeToken == "" {
		return fmt.Errorf("Linode token is required")
	}

	if len(c.Nodepools) == 0 {
		return fmt.Errorf("at least one nodepool must be configured")
	}

	for _, nodepool := range c.Nodepools {
		if nodepool.Name == "" {
			return fmt.Errorf("nodepool name cannot be empty")
		}
		if len(nodepool.Databases) == 0 {
			return fmt.Errorf("nodepool %s must have at least one database", nodepool.Name)
		}

		for _, db := range nodepool.Databases {
			if db.ID == "" {
				return fmt.Errorf("database ID cannot be empty in nodepool %s", nodepool.Name)
			}
			if db.Name == "" {
				return fmt.Errorf("database name cannot be empty in nodepool %s", nodepool.Name)
			}
		}
	}

	if c.NodeDeletionDelay <= 0 {
		return fmt.Errorf("node deletion delay must be a positive integer")
	}

	if c.APIRateLimit <= 0 {
		return fmt.Errorf("API rate limit must be a positive integer")
	}

	if c.Retry.MaxAttempts <= 0 {
		return fmt.Errorf("retry max attempts must be a positive integer")
	}

	if c.Retry.InitialBackoff < 0 {
		return fmt.Errorf("retry initial backoff must be non-negative")
	}

	if c.Retry.MaxBackoff < 0 {
		return fmt.Errorf("retry max backoff must be non-negative")
	}

	if c.Retry.InitialBackoff > c.Retry.MaxBackoff && c.Retry.MaxBackoff != 0 {
		return fmt.Errorf("retry max backoff must be greater than or equal to initial backoff")
	}

	return nil
} 