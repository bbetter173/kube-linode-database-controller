# Linode Database Allow List Manager

This Go application watches for Kubernetes Node Create/Delete events and updates Linode Managed Database allow_lists to include the external IPs of nodes in specific nodepools.

## Features

- Uses the client-go informer framework to watch for Node Create/Delete events
- Configurable label key for nodepool identification (defaults to 'lke.linode.com/pool-id')
- Updates Linode Managed Database allow_lists with node external IPs
- Supports IPv4, IPv6, or both IP versions for node address tracking
- Supports mapping between nodepool values and multiple databases
- Never removes existing IP addresses from database allow_lists
- Displays warnings when 0.0.0.0/0 is detected in an allow_list
- Implements retry logic with exponential backoff for API calls
- Exposes metrics for monitoring
- Supports leader election for high availability
- Handles node label changes and immediately removes access when nodes are removed from a nodepool

## Requirements

- Go 1.23 or higher
- Access to a Kubernetes cluster (v1.30+)
- Linode API token with appropriate permissions
- Kubernetes RBAC permissions to watch nodes

## Node Label Handling

The application now watches for node updates in addition to node additions and deletions. This enables immediate handling of label changes:

- When a node receives a label matching the configured nodepool label key, its IP is added to the corresponding database allow lists.
- When a node's label is removed or changed to an unwatched value, its IP is immediately removed from the database allow lists.
- When a node's label changes from one watched nodepool to another, its IP is removed from the old nodepool's database allow lists and added to the new ones.

The label key is configurable via the `nodepoolLabelKey` configuration option, environment variable `NODEPOOL_LABEL_KEY`, or command line flag `--nodepool-label-key`.

## IP Version Support

The application can be configured to track IPv4 addresses, IPv6 addresses, or both:

- By default, only IPv4 addresses are tracked and added to database allow lists
- When both are enabled, both types of addresses will be added to database allow lists
- At least one IP version must be enabled

IP version settings can be configured via:
- Configuration file settings `enableIPv4` and `enableIPv6`
- Environment variables `ENABLE_IPV4` and `ENABLE_IPV6` (values: "true" or "false")
- Helm chart values (see chart documentation)

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

# Log level (debug, info, warn, error, default: info)
logLevel: "info"

# Label key used to identify node pools (default: lke.linode.com/pool-id)
nodepoolLabelKey: "lke.linode.com/pool-id"

# IP version configuration (default: IPv4 only)
enableIPv4: true
enableIPv6: false
```

## Helm Chart

This repository includes a Helm chart for easy deployment to Kubernetes clusters. The chart is published to GitHub Pages and can be installed using Helm.

### Installing the Chart

```bash
# Add the repository
helm repo add bbetter173 https://bbetter173.github.io/kube-linode-database-controller/deployments/helm/charts

# Update the local repository cache
helm repo update

# Install the chart
helm install linode-db-allowlist bbetter173/linode-db-allowlist \
  --set linode.token.value=your-linode-api-token
```

For more detailed information about the Helm chart, see the [chart documentation](./deployments/helm/linode-db-allowlist/README.md).

### Environment Variables

The following environment variables can be used to configure the application:

- `LINODE_TOKEN`: Linode API token (required)
- `API_RATE_LIMIT`: API rate limit in requests per minute (default: 100)
- `LOG_LEVEL`: Log level (debug, info, warn, error, default: info)
- `NODEPOOL_LABEL_KEY`: Label key used to identify node pools (default: lke.linode.com/pool-id)
- `ENABLE_IPV4`: Whether to track IPv4 addresses (default: true)
- `ENABLE_IPV6`: Whether to track IPv6 addresses (default: false)

### Command Line Flags

The application accepts the following command line flags:

- `--config`: Path to configuration file
- `--kubeconfig`: Path to kubeconfig file for out-of-cluster operation
- `--metrics-addr`: Address to serve metrics on (default: ":8080")
- `--log-level`: Override log level (debug, info, warn, error)

Example:
```
./nodewatcher --config=/path/to/config.yaml --log-level=debug
```