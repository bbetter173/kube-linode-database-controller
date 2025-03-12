package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	"github.com/mediahq/linode-db-allowlist/pkg/kubernetes"
	"github.com/mediahq/linode-db-allowlist/pkg/linode"
	"github.com/mediahq/linode-db-allowlist/pkg/metrics"
	corev1 "k8s.io/api/core/v1"
	"go.uber.org/zap"
)

const (
	// DefaultMetricsAddr is the default address for the metrics server
	DefaultMetricsAddr = ":8080"
	
	// LeaseLockName is the name of the lease lock for leader election
	LeaseLockName = "linode-database-allow-list-lock"
	
	// LeaseLockNamespace is the namespace for the lease lock
	LeaseLockNamespace = "kube-system"
)

var (
	// Command line flags
	kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig file for out-of-cluster operation")
	configFile = flag.String("config", "", "Path to configuration file")
	metricsAddr = flag.String("metrics-addr", DefaultMetricsAddr, "Address to serve metrics on")
	logLevel = flag.String("log-level", "", "Override log level (debug, info, warn, error)")
)

func main() {
	// Parse command line flags
	flag.Parse()
	
	// Initialize logger - initially with default log level, will be updated after config is loaded
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	
	// Initialize configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}
	
	// Update logger with configured log level
	logger, err = initLogger(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to re-initialize logger with configured log level: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Log level set from configuration", zap.String("level", cfg.LogLevel))
	
	// Override log level from command-line flag if provided
	if *logLevel != "" {
		cfg.LogLevel = strings.ToLower(*logLevel)
		// Re-initialize logger with new log level
		logger, err = initLogger(cfg.LogLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to re-initialize logger: %v\n", err)
			os.Exit(1)
		}
		logger.Info("Log level overridden from command-line flag", zap.String("level", cfg.LogLevel))
	}
	
	// Initialize metrics
	metricsClient := metrics.NewMetrics(logger)
	
	// Start metrics server in a goroutine
	go func() {
		if err := metricsClient.StartMetricsServer(*metricsAddr); err != nil {
			logger.Fatal("Failed to start metrics server", zap.Error(err))
		}
	}()
	
	// Create Kubernetes client
	k8sClient, err := kubernetes.NewClient(logger, cfg, *kubeconfig, LeaseLockName, LeaseLockNamespace)
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}
	
	// Create Linode client
	linodeClient := linode.NewClient(logger, cfg, metricsClient)
	
	// Set up node event handlers
	k8sClient.RegisterNodeHandlers(
		// Add handler
		func(node *corev1.Node, eventType string) error {
			return handleNodeEvent(logger, k8sClient, linodeClient, metricsClient, node, "add", cfg)
		},
		// Delete handler
		func(node *corev1.Node, eventType string) error {
			return handleNodeEvent(logger, k8sClient, linodeClient, metricsClient, node, "delete", cfg)
		},
	)
	
	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		sig := <-sigCh
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()
	
	// Start Kubernetes watcher with leader election
	logger.Info("Starting node watcher")
	if err := k8sClient.Start(ctx); err != nil {
		logger.Fatal("Failed to start node watcher", zap.Error(err))
	}
	
	<-ctx.Done()
	logger.Info("Shutting down")
}

// handleNodeEvent processes node creation and deletion events
func handleNodeEvent(
	logger *zap.Logger,
	k8sClient *kubernetes.Client,
	linodeClient *linode.Client,
	metricsClient *metrics.Metrics,
	node *corev1.Node,
	eventType string,
	cfg *config.Config,
) error {
	nodeName := node.Name
	nodepoolName := k8sClient.GetNodepoolLabelValue(node)
	
	// Start timer for metrics
	startTime := time.Now()
	
	// Update metrics
	if eventType == "add" {
		metricsClient.IncrementNodeAdds(nodepoolName)
	} else {
		metricsClient.IncrementNodeDeletes(nodepoolName)
	}
	
	// Get external IP address
	ip, err := k8sClient.GetExternalIP(node)
	if err != nil {
		logger.Error("Failed to get external IP for node",
			zap.String("node", nodeName),
			zap.String("nodepool", nodepoolName),
			zap.Error(err),
		)
		metricsClient.IncrementNodeProcessingErrors(nodepoolName, eventType, "ip_extraction_failure")
		return err
	}
	
	// Update database allow lists
	logger.Info("Updating database allow lists",
		zap.String("node", nodeName),
		zap.String("nodepool", nodepoolName),
		zap.String("ip", ip),
		zap.String("operation", eventType),
	)
	
	err = linodeClient.UpdateAllowList(context.Background(), nodepoolName, nodeName, ip, eventType)
	if err != nil {
		logger.Error("Failed to update database allow lists",
			zap.String("node", nodeName),
			zap.String("nodepool", nodepoolName),
			zap.String("ip", ip),
			zap.String("operation", eventType),
			zap.Error(err),
		)
		
		// Update metrics
		errorType := "api_error"
		if strings.Contains(err.Error(), "no databases configured") {
			errorType = "configuration_error"
		}
		metricsClient.IncrementNodeProcessingErrors(nodepoolName, eventType, errorType)
		
		// Log Kubernetes event
		eventReason := "AllowListUpdateFailed"
		message := fmt.Sprintf("Failed to update database allow lists: %v", err)
		k8sClient.LogEvent(nodeName, "Warning", eventReason, message)
		
		return err
	}
	
	// Calculate processing duration for metrics
	duration := time.Since(startTime).Seconds()
	
	// Log success
	logger.Info("Successfully processed node event",
		zap.String("node", nodeName),
		zap.String("nodepool", nodepoolName),
		zap.String("ip", ip),
		zap.String("operation", eventType),
		zap.Float64("duration_seconds", duration),
	)
	
	// Log Kubernetes event
	eventReason := "AllowListUpdated"
	var message string
	if eventType == "add" {
		message = fmt.Sprintf("Added node IP %s to database allow lists", ip)
	} else {
		message = fmt.Sprintf("Scheduled removal of node IP %s from database allow lists", ip)
	}
	k8sClient.LogEvent(nodeName, "Normal", eventReason, message)
	
	// Update node count metrics
	updateNodeCountMetrics(k8sClient, metricsClient, cfg.Nodepools)
	
	return nil
}

// updateNodeCountMetrics updates metrics for node counts
func updateNodeCountMetrics(k8sClient *kubernetes.Client, metricsClient *metrics.Metrics, nodepools []config.Nodepool) {
	for _, nodepool := range nodepools {
		go func(npName string) {
			nodes, err := k8sClient.GetNodesByNodepool(npName)
			if err != nil {
				return
			}
			metricsClient.UpdateNodesWatched(npName, len(nodes))
		}(nodepool.Name)
	}
}

// initLogger initializes the logger
func initLogger(logLevel ...string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.DisableCaller = false
	config.DisableStacktrace = false
	
	// Use specified log level if provided
	if len(logLevel) > 0 && logLevel[0] != "" {
		switch logLevel[0] {
		case "debug":
			config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		case "info":
			config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		case "warn":
			config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
		case "error":
			config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		}
	} else if os.Getenv("DEBUG") == "true" {
		// For backward compatibility
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	
	return config.Build()
} 