# Linode Database Allow List Manager

This Go application watches for Kubernetes Node Create/Delete events and updates Linode Managed Database allow_lists to include the external IPs of nodes in specific nodepools.

## Features

- Uses the client-go informer framework to watch for Node Create/Delete events
- Filters nodes based on the 'mediahq.switch.tv/nodepool' label
- Updates Linode Managed Database allow_lists with node external IPs
- Supports mapping between nodepool values and multiple databases
- Never removes existing IP addresses from database allow_lists
- Displays warnings when 0.0.0.0/0 is detected in an allow_list
- Implements retry logic with exponential backoff for API calls
- Exposes metrics for monitoring
- Supports leader election for high availability
- Handles node deletion gracefully with configurable delay

## Requirements

- Go 1.21 or higher
- Access to a Kubernetes cluster (v1.30+)
- Linode API token with appropriate permissions
- Kubernetes RBAC permissions to watch nodes

## Configuration

Configuration can be provided via environment variables or a configuration file:

```yaml
# Configuration for nodepool to database mappings
nodepools:
  - name: "production"
    databases:
      - id: "123456"
        name: "production-db"
  - name: "staging"
    databases:
      - id: "789012"
        name: "staging-db"

# API rate limit (requests per minute, default: 100)
apiRateLimit: 100

# Retry configuration
retry:
  maxAttempts: 5
  initialBackoff: 1s
  maxBackoff: 30s
```