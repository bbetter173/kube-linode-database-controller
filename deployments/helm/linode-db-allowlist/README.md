# Linode Database Allow List Helm Chart

This Helm chart deploys the Linode Database Allow List Manager, an application that watches for Kubernetes Node events and updates Linode Managed Database allow_lists accordingly.

## Prerequisites

- Kubernetes 1.30+
- Helm 3.0+
- A Linode API token with permissions to manage managed databases

## Installing the Chart

To install the chart with the release name `my-release`:

```bash
# Add the repository
helm repo add mediahq https://mediahq.github.io/linode-db-allowlist/helm/charts

# Update the local repository cache
helm repo update

# Install the chart
helm install my-release mediahq/linode-db-allowlist \
  --set linode.token.value=your-linode-api-token
```

> **Tip**: It's recommended to create a Secret with your Linode API token and use `linode.token.existingSecret` instead.

## Configuration

The following table lists the configurable parameters of the Linode Database Allow List chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas to deploy | `2` |
| `image.repository` | Image repository | `mediahq/linode-db-allowlist` |
| `image.tag` | Image tag | `""` (defaults to Chart.appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `nameOverride` | Override the chart name | `""` |
| `fullnameOverride` | Override the full chart name | `""` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Kubernetes service port | `8080` |
| `resources.limits.cpu` | CPU resource limits | `500m` |
| `resources.limits.memory` | Memory resource limits | `256Mi` |
| `resources.requests.cpu` | CPU resource requests | `100m` |
| `resources.requests.memory` | Memory resource requests | `128Mi` |
| `linode.token.existingSecret` | Name of existing secret | `""` |
| `linode.token.secretKey` | Key in the existing secret | `LINODE_TOKEN` |
| `linode.token.value` | Linode API token | `""` |
| `config.logLevel` | Log level (debug, info, warn, error) | `info` |
| `config.nodepoolLabelKey` | Label key used to identify node pools | `lke.linode.com/pool-id` |
| `config.apiRateLimit` | API rate limit in requests per minute | `100` |
| `config.retry.maxAttempts` | Maximum retry attempts | `5` |
| `config.retry.initialBackoff` | Initial backoff duration | `1s` |
| `config.retry.maxBackoff` | Maximum backoff duration | `30s` |
| `config.nodepools` | Nodepool to database mappings | See values.yaml |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.serviceMonitor.enabled` | Enable ServiceMonitor for Prometheus Operator | `false` |
| `metrics.serviceMonitor.additionalLabels` | Additional labels for ServiceMonitor | `{}` |
| `metrics.serviceMonitor.interval` | ServiceMonitor scrape interval | `15s` |
| `metrics.serviceMonitor.scrapeTimeout` | ServiceMonitor scrape timeout | `5s` |
| `rbac.create` | Create RBAC resources | `true` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` |
| `serviceAccount.annotations` | Service account annotations | `{}` |

## Example: Using an Existing Secret

Create a Secret with your Linode API token:

```bash
kubectl create secret generic linode-token --from-literal=LINODE_TOKEN=your-linode-api-token
```

Then, install the chart referring to this Secret:

```bash
helm install my-release mediahq/linode-db-allowlist \
  --set linode.token.existingSecret=linode-token
```

## Example: Configure Nodepool Mappings

```yaml
config:
  nodepools:
    - name: "production"
      databases:
        - id: "123456"
          name: "production-db"
        - id: "654321"
          name: "shared-db"
    - name: "staging"
      databases:
        - id: "789012"
          name: "staging-db"
```

## Upgrading the Chart

To upgrade the release `my-release`:

```bash
helm upgrade my-release mediahq/linode-db-allowlist
```

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```bash
helm delete my-release
``` 