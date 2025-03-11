package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Metrics contains Prometheus metrics for the application
type Metrics struct {
	Registry             *prometheus.Registry
	logger               *zap.Logger
	mutex                sync.Mutex
	
	// Node metrics
	NodesWatched         *prometheus.GaugeVec
	NodeAddsTotal        *prometheus.CounterVec
	NodeDeletesTotal     *prometheus.CounterVec
	NodeProcessingErrors *prometheus.CounterVec
	
	// Database metrics
	AllowListUpdatesTotal     *prometheus.CounterVec
	AllowListUpdateErrors     *prometheus.CounterVec
	AllowListUpdateLatency    *prometheus.HistogramVec
	AllowListOpenAccessAlerts *prometheus.CounterVec
	
	// API metrics
	APIRateLimitRemaining  *prometheus.GaugeVec
	APIRequestsTotal       *prometheus.CounterVec
	
	// Retry metrics
	RetriesTotal           *prometheus.CounterVec
	RetryFailuresTotal     *prometheus.CounterVec
	
	// Application metrics
	LeaderStatus           prometheus.Gauge
	PendingDeletions       *prometheus.GaugeVec
}

// NewMetrics creates a new Metrics instance
func NewMetrics(logger *zap.Logger) *Metrics {
	registry := prometheus.NewRegistry()
	
	metrics := &Metrics{
		Registry: registry,
		logger:   logger,
		
		NodesWatched: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "nodes_watched",
				Help: "Number of nodes being watched per nodepool",
			},
			[]string{"nodepool"},
		),
		
		NodeAddsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "node_adds_total",
				Help: "Total number of node additions processed",
			},
			[]string{"nodepool"},
		),
		
		NodeDeletesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "node_deletes_total",
				Help: "Total number of node deletions processed",
			},
			[]string{"nodepool"},
		),
		
		NodeProcessingErrors: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "node_processing_errors_total",
				Help: "Total number of errors processing node events",
			},
			[]string{"nodepool", "operation", "error_type"},
		),
		
		AllowListUpdatesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "allow_list_updates_total",
				Help: "Total number of database allow list updates",
			},
			[]string{"database", "operation"},
		),
		
		AllowListUpdateErrors: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "allow_list_update_errors_total",
				Help: "Total number of database allow list update errors",
			},
			[]string{"database", "operation", "error_type"},
		),
		
		AllowListUpdateLatency: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "allow_list_update_latency_seconds",
				Help:    "Latency of database allow list updates in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"database", "operation"},
		),
		
		AllowListOpenAccessAlerts: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "allow_list_open_access_alerts_total",
				Help: "Total number of alerts for 0.0.0.0/0 in database allow lists",
			},
			[]string{"database"},
		),
		
		APIRateLimitRemaining: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "api_rate_limit_remaining",
				Help: "Remaining API rate limit",
			},
			[]string{"api"},
		),
		
		APIRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_requests_total",
				Help: "Total number of API requests",
			},
			[]string{"api", "endpoint", "status"},
		),
		
		RetriesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "retries_total",
				Help: "Total number of operation retries",
			},
			[]string{"operation"},
		),
		
		RetryFailuresTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "retry_failures_total",
				Help: "Total number of operation retry failures",
			},
			[]string{"operation"},
		),
		
		LeaderStatus: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "leader_status",
				Help: "Leader status (1=leader, 0=follower)",
			},
		),
		
		PendingDeletions: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pending_deletions",
				Help: "Number of pending IP deletion operations",
			},
			[]string{"nodepool"},
		),
	}
	
	return metrics
}

// StartMetricsServer starts the Prometheus metrics server
func (m *Metrics) StartMetricsServer(addr string) error {
	mux := http.NewServeMux()
	
	// Register metrics handler
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	
	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	
	// Start HTTP server
	m.logger.Info("Starting metrics server", zap.String("addr", addr))
	return http.ListenAndServe(addr, mux)
}

// UpdateNodesWatched updates the NodesWatched gauge for a nodepool
func (m *Metrics) UpdateNodesWatched(nodepool string, count int) {
	m.NodesWatched.WithLabelValues(nodepool).Set(float64(count))
}

// IncrementNodeAdds increments the NodeAddsTotal counter for a nodepool
func (m *Metrics) IncrementNodeAdds(nodepool string) {
	m.NodeAddsTotal.WithLabelValues(nodepool).Inc()
}

// IncrementNodeDeletes increments the NodeDeletesTotal counter for a nodepool
func (m *Metrics) IncrementNodeDeletes(nodepool string) {
	m.NodeDeletesTotal.WithLabelValues(nodepool).Inc()
}

// IncrementNodeProcessingErrors increments the NodeProcessingErrors counter
func (m *Metrics) IncrementNodeProcessingErrors(nodepool, operation, errorType string) {
	m.NodeProcessingErrors.WithLabelValues(nodepool, operation, errorType).Inc()
}

// IncrementAllowListUpdates increments the AllowListUpdatesTotal counter
func (m *Metrics) IncrementAllowListUpdates(database, operation string) {
	m.AllowListUpdatesTotal.WithLabelValues(database, operation).Inc()
}

// IncrementAllowListUpdateErrors increments the AllowListUpdateErrors counter
func (m *Metrics) IncrementAllowListUpdateErrors(database, operation, errorType string) {
	m.AllowListUpdateErrors.WithLabelValues(database, operation, errorType).Inc()
}

// ObserveAllowListUpdateLatency records the latency of a database allow list update
func (m *Metrics) ObserveAllowListUpdateLatency(database, operation string, latencySeconds float64) {
	m.AllowListUpdateLatency.WithLabelValues(database, operation).Observe(latencySeconds)
}

// IncrementAllowListOpenAccessAlerts increments the AllowListOpenAccessAlerts counter
func (m *Metrics) IncrementAllowListOpenAccessAlerts(database string) {
	m.AllowListOpenAccessAlerts.WithLabelValues(database).Inc()
}

// UpdateAPIRateLimitRemaining updates the APIRateLimitRemaining gauge
func (m *Metrics) UpdateAPIRateLimitRemaining(api string, remaining float64) {
	m.APIRateLimitRemaining.WithLabelValues(api).Set(remaining)
}

// IncrementAPIRequests increments the APIRequestsTotal counter
func (m *Metrics) IncrementAPIRequests(api, endpoint, status string) {
	m.APIRequestsTotal.WithLabelValues(api, endpoint, status).Inc()
}

// IncrementRetries increments the RetriesTotal counter
func (m *Metrics) IncrementRetries(operation string) {
	m.RetriesTotal.WithLabelValues(operation).Inc()
}

// IncrementRetryFailures increments the RetryFailuresTotal counter
func (m *Metrics) IncrementRetryFailures(operation string) {
	m.RetryFailuresTotal.WithLabelValues(operation).Inc()
}

// SetLeaderStatus sets the LeaderStatus gauge
func (m *Metrics) SetLeaderStatus(isLeader bool) {
	value := 0.0
	if isLeader {
		value = 1.0
	}
	m.LeaderStatus.Set(value)
}

// UpdatePendingDeletions updates the PendingDeletions gauge
func (m *Metrics) UpdatePendingDeletions(nodepool string, count int) {
	m.PendingDeletions.WithLabelValues(nodepool).Set(float64(count))
}

// RegisterMetrics registers all the metrics with the global Prometheus registry
// This method is for backward compatibility and doesn't need to do anything 
// since the metrics are already registered with the Registry in the constructor
func (m *Metrics) RegisterMetrics() {
	// All metrics are already registered during initialization
	// This method exists for backward compatibility
}

// Handler returns a HTTP handler for serving Prometheus metrics
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

// IncrementNodeCount increments the node count counter for a nodepool
func (m *Metrics) IncrementNodeCount(nodepool string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	current := m.NodesWatched.WithLabelValues(nodepool)
	currentValue := testutil.ToFloat64(current)
	current.Set(currentValue + 1)
}

// DecrementNodeCount decrements the node count counter for a nodepool, but not below 0
func (m *Metrics) DecrementNodeCount(nodepool string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	current := m.NodesWatched.WithLabelValues(nodepool)
	currentValue := testutil.ToFloat64(current)
	if currentValue > 0 {
		current.Set(currentValue - 1)
	}
}

// SetNodeCount sets the node count counter for a nodepool
func (m *Metrics) SetNodeCount(nodepool string, count int) {
	m.NodesWatched.WithLabelValues(nodepool).Set(float64(count))
}

// ObserveOperationDuration records the duration of an operation
func (m *Metrics) ObserveOperationDuration(operation string, result string, durationSeconds float64) {
	// This is just a stub method for backward compatibility
	// Operation duration is now tracked via specific histogram metrics
} 