package linode

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/linode/linodego"
	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"github.com/mediahq/linode-db-allowlist/pkg/utils"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

// AllowAnyIP is a constant for the 0.0.0.0/0 IP range that allows access from any IP
const AllowAnyIP = "0.0.0.0/0"

// DatabaseOperation represents an operation on a database allow list
type DatabaseOperation struct {
	DatabaseID string
	NodeName   string
	IP         string
	Operation  string // "add" or "remove"
	Timestamp  time.Time
}

// LinodeAPI defines the Linode API operations needed by this application
type LinodeAPI interface {
	GetDatabaseAllowList(ctx context.Context, dbID string) ([]string, error)
	UpdateDatabaseAllowList(ctx context.Context, dbID string, allowList []string) error
}

// MetricsClient defines the metrics operations needed by this application
type MetricsClient interface {
	IncrementAllowListUpdates(database, operation string)
	ObserveAllowListUpdateLatency(database, operation string, latencySeconds float64)
	UpdatePendingDeletions(nodepool string, count int)
	UpdateAPIRateLimitRemaining(api string, remaining float64)
	IncrementAllowListOpenAccessAlerts(database string)
}

// LinodeClient defines the operations of the Linode client
type LinodeClient interface {
	// UpdateAllowList adds or removes an IP to/from database allow lists associated with a nodepool
	UpdateAllowList(ctx context.Context, nodepoolName, nodeName, ip, operation string) error
}

// LinodeAPIAdapter adapts linodego.Client to the LinodeAPI interface
type LinodeAPIAdapter struct {
	client linodego.Client
}

// GetDatabaseAllowList implements the LinodeAPI interface
func (a *LinodeAPIAdapter) GetDatabaseAllowList(ctx context.Context, dbID string) ([]string, error) {
	// Convert dbID from string to int
	dbIDInt, err := strconv.Atoi(dbID)
	if err != nil {
		return nil, fmt.Errorf("invalid database ID %s: %w", dbID, err)
	}
	
	// Get the database with the specified ID
	database, err := a.client.GetMySQLDatabase(ctx, dbIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to get database %s: %w", dbID, err)
	}
	
	// Return the allow list from the database
	return database.AllowList, nil
}

// UpdateDatabaseAllowList implements the LinodeAPI interface
func (a *LinodeAPIAdapter) UpdateDatabaseAllowList(ctx context.Context, dbID string, allowList []string) error {
	// Convert dbID from string to int
	dbIDInt, err := strconv.Atoi(dbID)
	if err != nil {
		return fmt.Errorf("invalid database ID %s: %w", dbID, err)
	}
	
	// Create the update options with the new allow list
	// AllowList needs to be a pointer to []string
	allowListCopy := allowList
	updateOpts := linodego.MySQLUpdateOptions{
		AllowList: &allowListCopy,
	}
	
	// Update the database
	_, err = a.client.UpdateMySQLDatabase(ctx, dbIDInt, updateOpts)
	if err != nil {
		return fmt.Errorf("failed to update database %s allow list: %w", dbID, err)
	}
	
	return nil
}

// Client manages Linode API connectivity and implements LinodeClient
type Client struct {
	api              LinodeAPI
	logger           *zap.Logger
	config           *config.Config
	metrics          MetricsClient
	mutex            sync.Mutex
	rateLimiter      *rate.Limiter
}

// NewClient creates a new LinodeClient
func NewClient(logger *zap.Logger, cfg *config.Config, metricsClient MetricsClient) *Client {
	// Create OAuth2 token source
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: cfg.LinodeToken,
	})
	
	// Create OAuth2 HTTP client
	oauth2Client := oauth2.NewClient(context.Background(), tokenSource)
	
	// Create Linode API client
	linodeClient := linodego.NewClient(oauth2Client)
	
	// Wrap the linodego client with our adapter
	apiAdapter := &LinodeAPIAdapter{
		client: linodeClient,
	}
	
	// Calculate requests per second based on rate limit
	rps := float64(cfg.APIRateLimit) / 60.0
	
	return &Client{
		api:              apiAdapter,
		logger:           logger,
		config:           cfg,
		metrics:          metricsClient,
		rateLimiter:      rate.NewLimiter(rate.Limit(rps), 1),
	}
}

// NewClientWithAPI creates a new LinodeClient with a custom API implementation
func NewClientWithAPI(logger *zap.Logger, cfg *config.Config, api LinodeAPI, metricsClient MetricsClient) *Client {
	// Calculate requests per second based on rate limit
	rps := float64(cfg.APIRateLimit) / 60.0
	
	return &Client{
		api:              api,
		logger:           logger,
		config:           cfg,
		metrics:          metricsClient,
		rateLimiter:      rate.NewLimiter(rate.Limit(rps), 1),
	}
}

// UpdateAllowList updates the database allow list for all databases associated with a nodepool
func (c *Client) UpdateAllowList(ctx context.Context, nodepoolName, nodeName, ip, operation string) error {
	// Find databases for this nodepool
	var databases []config.Database
	for _, np := range c.config.Nodepools {
		if np.Name == nodepoolName {
			databases = np.Databases
			break
		}
	}

	if len(databases) == 0 {
		return fmt.Errorf("no databases configured for nodepool %s", nodepoolName)
	}

	// Handle add operation
	if operation == "add" {
		return c.handleAddOperation(ctx, nodepoolName, databases, nodeName, ip)
	} 
	
	// Handle delete operation
	return c.handleDeleteOperation(ctx, nodepoolName, databases, nodeName, ip)
}

// handleAddOperation adds an IP to all databases associated with a nodepool
func (c *Client) handleAddOperation(ctx context.Context, nodepoolName string, databases []config.Database, nodeName, ip string) error {
	// Update all databases associated with the nodepool
	var lastErr error
	
	for _, db := range databases {
		if err := c.updateDatabaseAllowList(ctx, db.ID, db.Name, nodeName, ip, "add"); err != nil {
			c.logger.Error("Failed to add IP to database allow list",
				zap.String("database", db.Name),
				zap.String("ip", ip),
				zap.String("node", nodeName),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	
	return lastErr
}

// handleDeleteOperation immediately removes an IP from database allow lists
func (c *Client) handleDeleteOperation(ctx context.Context, nodepoolName string, databases []config.Database, nodeName, ip string) error {
	// Update the pending deletions metric - set to 0 for immediate deletion
	c.metrics.UpdatePendingDeletions(nodepoolName, 0)
	
	// Check if IP is still in use by other nodes in the same nodepool
	stillInUse, err := c.isIPStillInUseByNodepool(ctx, nodepoolName, ip)
	if err != nil {
		c.logger.Error("Failed to check if IP is still in use",
			zap.String("nodepool", nodepoolName),
			zap.String("ip", ip),
			zap.Error(err),
		)
		// Don't delete if we can't verify it's safe
		return err
	}

	if stillInUse {
		c.logger.Info("Skipping deletion because IP is still in use by other nodes",
			zap.String("nodepool", nodepoolName),
			zap.String("ip", ip),
		)
		return nil
	}
	
	var lastErr error
	for _, db := range databases {
		if err := c.updateDatabaseAllowList(ctx, db.ID, db.Name, nodeName, ip, "remove"); err != nil {
			c.logger.Error("Failed to remove IP from database allow list",
				zap.String("database", db.Name),
				zap.String("ip", ip),
				zap.String("node", nodeName),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	return lastErr
}

// Helper function to get databases for a nodepool
func getDatabasesForNodepool(cfg *config.Config, nodepoolName string) []config.Database {
	for _, np := range cfg.Nodepools {
		if np.Name == nodepoolName {
			return np.Databases
		}
	}
	return nil
}

// This function updates a database allow list by adding or removing an IP address
func (c *Client) updateDatabaseAllowList(ctx context.Context, dbID, dbName, nodeName, ip, operation string) error {
	// Wrap with metrics
	startTime := time.Now()
	defer func() {
		elapsedSeconds := time.Since(startTime).Seconds()
		c.metrics.ObserveAllowListUpdateLatency(dbName, operation, elapsedSeconds)
	}()

	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}

	// Use retry for reliability
	return utils.RetryWithBackoff(ctx, c.logger, c.config.Retry, "update_allow_list", func() error {
		// Get current allow list
		allowList, err := c.api.GetDatabaseAllowList(ctx, dbID)
		if err != nil {
			return err
		}

		// Check for 0.0.0.0/0 in the allow list
		for _, entry := range allowList {
			if entry == "0.0.0.0/0" || entry == "::/0" {
				c.logger.Warn("Open access detected in database allow list",
					zap.String("database", dbName),
					zap.String("entry", entry),
				)
				c.metrics.IncrementAllowListOpenAccessAlerts(dbName)
			}
		}

		// Convert to a map for easy manipulation
		ipSet := make(map[string]bool)
		for _, entry := range allowList {
			ipSet[entry] = true
		}

		// Add or remove the IP
		modified := false

		if operation == "add" {
			if !ipSet[ip] {
				ipSet[ip] = true
				modified = true
				c.logger.Info("Adding IP to database allow list",
					zap.String("database", dbName),
					zap.String("ip", ip),
					zap.String("node", nodeName),
				)
			} else {
				c.logger.Debug("IP already in database allow list",
					zap.String("database", dbName),
					zap.String("ip", ip),
				)
			}
		} else if operation == "remove" {
			if ipSet[ip] {
				delete(ipSet, ip)
				modified = true
				c.logger.Info("Removing IP from database allow list",
					zap.String("database", dbName),
					zap.String("ip", ip),
					zap.String("node", nodeName),
				)
			} else {
				c.logger.Debug("IP not in database allow list",
					zap.String("database", dbName),
					zap.String("ip", ip),
				)
			}
		}

		// If no changes, return
		if !modified {
			return nil
		}

		// Convert back to slice and sort
		newAllowList := make([]string, 0, len(ipSet))
		for entry := range ipSet {
			newAllowList = append(newAllowList, entry)
		}
		sort.Strings(newAllowList)

		// Update the allow list
		err = c.api.UpdateDatabaseAllowList(ctx, dbID, newAllowList)
		if err == nil {
			c.metrics.IncrementAllowListUpdates(dbName, operation)
		}
		return err
	}, utils.DefaultIsRetryable)
}

// isIPStillInUseByNodepool checks if an IP is still used by any node in the nodepool
func (c *Client) isIPStillInUseByNodepool(ctx context.Context, nodepoolName, ip string) (bool, error) {
	// This is a placeholder for the actual implementation
	// In a real implementation, you would:
	// 1. Query the Kubernetes API for all nodes in the nodepool
	// 2. Check if any node has the IP address
	// 3. Return true if found, false otherwise
	
	// For now, always return false (meaning the IP is not in use)
	// This should be implemented with the actual Kubernetes client
	return false, nil
} 