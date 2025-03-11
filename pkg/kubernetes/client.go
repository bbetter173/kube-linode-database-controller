package kubernetes

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/mediahq/linode-db-allowlist/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"go.uber.org/zap"
)

const (
	// NodePoolLabelKey is the label key used to identify node pools
	NodePoolLabelKey = "mediahq.switch.tv/nodepool"
	
	// ResyncPeriod is how often the informer does a full resync
	ResyncPeriod = 30 * time.Minute
	
	// LeaseDuration is how long a lease is valid
	LeaseDuration = 15 * time.Second
	
	// RenewDeadline is how long the leader has to renew the lease
	RenewDeadline = 10 * time.Second
	
	// RetryPeriod is how often to retry acquiring the lease
	RetryPeriod = 2 * time.Second
)

// NodeHandler is a callback function for node events
type NodeHandler func(node *corev1.Node, eventType string) error

// Client manages Kubernetes connectivity and node watching
type Client struct {
	clientset      *kubernetes.Clientset
	logger         *zap.Logger
	config         *config.Config
	nodeAddHandler NodeHandler
	nodeDelHandler NodeHandler
	stopCh         chan struct{}
	leaseLockName  string
	leaseLockNamespace string
}

// NewClient creates a new Kubernetes client
func NewClient(logger *zap.Logger, cfg *config.Config, kubeconfigPath string, leaseLockName, leaseLockNamespace string) (*Client, error) {
	var kubeConfig *rest.Config
	var err error

	if kubeconfigPath == "" {
		// In-cluster configuration
		logger.Info("Using in-cluster configuration")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else {
		// Out-of-cluster configuration
		logger.Info("Using provided kubeconfig", zap.String("path", kubeconfigPath))
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &Client{
		clientset:      clientset,
		logger:         logger,
		config:         cfg,
		stopCh:         make(chan struct{}),
		leaseLockName:  leaseLockName,
		leaseLockNamespace: leaseLockNamespace,
	}, nil
}

// RegisterNodeHandlers sets the callback functions for node events
func (c *Client) RegisterNodeHandlers(addHandler, deleteHandler NodeHandler) {
	c.nodeAddHandler = addHandler
	c.nodeDelHandler = deleteHandler
}

// Start begins watching nodes with leader election
func (c *Client) Start(ctx context.Context) error {
	if c.nodeAddHandler == nil || c.nodeDelHandler == nil {
		return fmt.Errorf("node handlers are not registered")
	}

	// Create a new lock
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      c.leaseLockName,
			Namespace: c.leaseLockNamespace,
		},
		Client: c.clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: getHostname(),
		},
	}

	// Start leader election
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   LeaseDuration,
		RenewDeadline:   RenewDeadline,
		RetryPeriod:     RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				c.logger.Info("Started leading, watching nodes")
				if err := c.watchNodes(ctx); err != nil {
					c.logger.Error("Error watching nodes", zap.Error(err))
				}
			},
			OnStoppedLeading: func() {
				c.logger.Info("Stopped leading")
				close(c.stopCh)
			},
			OnNewLeader: func(identity string) {
				if identity != getHostname() {
					c.logger.Info("New leader elected", zap.String("leader", identity))
				}
			},
		},
	})

	return nil
}

// Stop stops watching for node events
func (c *Client) Stop() {
	close(c.stopCh)
}

// watchNodes sets up the informer for node events
func (c *Client) watchNodes(ctx context.Context) error {
	// Create shared informer factory with custom resync period and specific options
	// Use a more optimized set of options to ensure the watch doesn't miss events
	factory := informers.NewSharedInformerFactoryWithOptions(
		c.clientset,
		ResyncPeriod,
		// Use a node-specific tweak to ensure we get all events
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// Set a reasonable timeout for the watch (5 minutes)
			timeoutSeconds := int64(300)
			options.TimeoutSeconds = &timeoutSeconds
			// Use a field selector to ensure we get all nodes
			options.FieldSelector = ""
			// Set explicit resource version to ensure we don't miss events
			options.ResourceVersion = ""
			// Allow watching from the beginning
			options.AllowWatchBookmarks = true
		}),
	)

	// Create node informer
	nodeInformer := factory.Core().V1().Nodes().Informer()
	
	// Add safer reflection for watch error handling - use a defensive approach
	c.setupWatchErrorHandler(nodeInformer)

	// Set up event handlers
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
			c.logger.Debug("Received node add event from watch", 
				zap.String("node", node.Name),
				zap.String("labels", fmt.Sprintf("%v", node.Labels)),
			)
			if c.isNodeInWatchedNodepool(node) {
				c.logger.Info("Node added",
					zap.String("node", node.Name),
					zap.String("nodepool", c.getNodepoolName(node)),
				)
				if err := c.nodeAddHandler(node, "add"); err != nil {
					c.logger.Error("Error handling node add event",
						zap.String("node", node.Name),
						zap.Error(err),
					)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode := oldObj.(*corev1.Node)
			newNode := newObj.(*corev1.Node)
			
			c.logger.Debug("Received node update event from watch", 
				zap.String("node", newNode.Name),
				zap.String("old_labels", fmt.Sprintf("%v", oldNode.Labels)),
				zap.String("new_labels", fmt.Sprintf("%v", newNode.Labels)),
			)
			
			// Check if this is a node that was just added to or removed from a watched nodepool
			oldNodepool := c.getNodepoolName(oldNode)
			newNodepool := c.getNodepoolName(newNode)
			
			// If the node was added to a watched nodepool, treat it as an Add
			oldIsWatched := c.isNodeInWatchedNodepool(oldNode)
			newIsWatched := c.isNodeInWatchedNodepool(newNode)
			
			if !oldIsWatched && newIsWatched {
				c.logger.Info("Node added to watched nodepool",
					zap.String("node", newNode.Name),
					zap.String("old_nodepool", oldNodepool),
					zap.String("new_nodepool", newNodepool),
				)
				if err := c.nodeAddHandler(newNode, "add"); err != nil {
					c.logger.Error("Error handling node add to nodepool event",
						zap.String("node", newNode.Name),
						zap.Error(err),
					)
				}
			} else if oldIsWatched && !newIsWatched {
				// If the node was removed from a watched nodepool (including label removal), treat it as a Delete
				c.logger.Info("Node removed from watched nodepool",
					zap.String("node", newNode.Name),
					zap.String("old_nodepool", oldNodepool),
					zap.String("new_nodepool", newNodepool),
					zap.Bool("label_removed", newNodepool == ""),
				)
				if err := c.nodeDelHandler(oldNode, "delete"); err != nil {
					c.logger.Error("Error handling node remove from nodepool event",
						zap.String("node", oldNode.Name),
						zap.Error(err),
					)
				}
			} else if oldIsWatched && newIsWatched && oldNodepool != newNodepool {
				// Handle the case where the node moved between watched nodepools
				c.logger.Info("Node moved between watched nodepools",
					zap.String("node", newNode.Name),
					zap.String("old_nodepool", oldNodepool),
					zap.String("new_nodepool", newNodepool),
				)
				
				// First delete from old nodepool
				if err := c.nodeDelHandler(oldNode, "delete"); err != nil {
					c.logger.Error("Error handling node removal from old nodepool",
						zap.String("node", oldNode.Name),
						zap.String("old_nodepool", oldNodepool),
						zap.Error(err),
					)
				}
				
				// Then add to new nodepool
				if err := c.nodeAddHandler(newNode, "add"); err != nil {
					c.logger.Error("Error handling node addition to new nodepool",
						zap.String("node", newNode.Name),
						zap.String("new_nodepool", newNodepool),
						zap.Error(err),
					)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
			c.logger.Debug("Received node delete event from watch", 
				zap.String("node", node.Name),
				zap.String("labels", fmt.Sprintf("%v", node.Labels)),
			)
			if c.isNodeInWatchedNodepool(node) {
				c.logger.Info("Node deleted",
					zap.String("node", node.Name),
					zap.String("nodepool", c.getNodepoolName(node)),
				)
				if err := c.nodeDelHandler(node, "delete"); err != nil {
					c.logger.Error("Error handling node delete event",
						zap.String("node", node.Name),
						zap.Error(err),
					)
				}
			}
		},
	})

	// Start informer
	c.logger.Info("Starting node informer")
	factory.Start(c.stopCh)
	
	// Wait for cache sync
	c.logger.Info("Waiting for node informer cache to sync")
	if !cache.WaitForCacheSync(c.stopCh, nodeInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for node informer cache to sync")
	}
	c.logger.Info("Node informer cache synced successfully")
	
	// Run an initial verification after a brief delay to ensure the watch is working
	go func() {
		// Wait 10 seconds before first verification to allow the watch to stabilize
		time.Sleep(10 * time.Second)
		c.logger.Info("Performing initial watch connection verification")
		consistent, _ := c.verifyWatchConsistency(ctx)
		if !consistent {
			c.logger.Warn("Initial watch verification failed - watch may not be catching all events. " +
				"Check for network issues, API server constraints, or firewall rules.")
		} else {
			c.logger.Info("Initial watch verification successful - watch appears to be working correctly")
		}
	}()
	
	// Periodically log that we're still watching to confirm the informer is active
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		
		var consecutiveMismatches int
		
		for {
			select {
			case <-ticker.C:
				// Log that the informer is active
				c.logger.Info("Node informer still active")
				
				// Verify the watch connection by checking consistency
				consistent, _ := c.verifyWatchConsistency(ctx)
				if !consistent {
					consecutiveMismatches++
					if consecutiveMismatches >= 2 {
						c.logger.Error("Watch appears to be broken after multiple verification failures. Kubernetes node events may be missed.",
							zap.Int("consecutive_mismatches", consecutiveMismatches),
						)
						// We would reset the watch here, but this would require a significant refactoring
						// of the informer pattern. We'll rely on the debug info to help diagnose instead.
					}
				} else {
					consecutiveMismatches = 0
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	
	// Block until stop channel is closed
	<-ctx.Done()
	c.logger.Info("Node informer stopped")
	return nil
}

// verifyWatchConnection verifies the watch connection by listing nodes directly
// and comparing with what's in our cache
func (c *Client) verifyWatchConnection(ctx context.Context) {
	// List nodes directly from the API
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		c.logger.Error("Failed to list nodes when verifying watch connection", zap.Error(err))
		return
	}
	
	// Count nodes by nodepool for verification
	nodepoolCounts := make(map[string]int)
	watchedNodepools := make(map[string]bool)
	
	// Build list of watched nodepools for easy lookup
	for _, np := range c.config.Nodepools {
		watchedNodepools[np.Name] = true
	}
	
	// Count nodes in each nodepool
	for _, node := range nodes.Items {
		nodepoolName := c.getNodepoolName(&node)
		if nodepoolName != "" {
			nodepoolCounts[nodepoolName]++
		}
	}
	
	// Log counts for watched nodepools
	for nodepool, count := range nodepoolCounts {
		if watchedNodepools[nodepool] {
			c.logger.Info("Current node count from direct API query", 
				zap.String("nodepool", nodepool), 
				zap.Int("count", count),
			)
		}
	}
	
	// Now verify against our informer cache by using GetNodesByNodepool
	for nodepool := range watchedNodepools {
		cacheNodes, err := c.GetNodesByNodepool(nodepool)
		if err != nil {
			c.logger.Error("Failed to get nodes from cache when verifying", 
				zap.String("nodepool", nodepool),
				zap.Error(err),
			)
			continue
		}
		
		apiCount := nodepoolCounts[nodepool]
		cacheCount := len(cacheNodes)
		
		if apiCount != cacheCount {
			c.logger.Warn("Watch connection may have issues - node count mismatch",
				zap.String("nodepool", nodepool),
				zap.Int("api_count", apiCount),
				zap.Int("cache_count", cacheCount),
			)
			// Log the node names from both to help diagnose
			apiNodeNames := make([]string, 0, len(nodes.Items))
			for _, node := range nodes.Items {
				if c.getNodepoolName(&node) == nodepool {
					apiNodeNames = append(apiNodeNames, node.Name)
				}
			}
			
			cacheNodeNames := make([]string, 0, len(cacheNodes))
			for _, node := range cacheNodes {
				cacheNodeNames = append(cacheNodeNames, node.Name)
			}
			
			c.logger.Warn("Node list comparison",
				zap.String("nodepool", nodepool),
				zap.Strings("api_nodes", apiNodeNames),
				zap.Strings("cache_nodes", cacheNodeNames),
			)
		} else {
			c.logger.Info("Watch connection verification successful",
				zap.String("nodepool", nodepool),
				zap.Int("node_count", apiCount),
			)
		}
	}
}

// isNodeInWatchedNodepool checks if a node belongs to a watched nodepool
func (c *Client) isNodeInWatchedNodepool(node *corev1.Node) bool {
	nodepoolName := c.getNodepoolName(node)
	if nodepoolName == "" {
		return false
	}

	// Check if the nodepool is in our configuration
	for _, np := range c.config.Nodepools {
		if np.Name == nodepoolName {
			return true
		}
	}

	return false
}

// getNodepoolName extracts the nodepool name from a node
func (c *Client) getNodepoolName(node *corev1.Node) string {
	if node == nil || node.Labels == nil {
		return ""
	}
	
	return node.Labels[c.config.NodepoolLabelKey]
}

// GetExternalIP gets the external IP address of a node
func (c *Client) GetExternalIP(node *corev1.Node) (string, error) {
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeExternalIP {
			return address.Address, nil
		}
	}

	// Fallback to internal IP if external is not available
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			c.logger.Warn("Node has no external IP, using internal IP",
				zap.String("node", node.Name),
				zap.String("ip", address.Address),
			)
			return address.Address, nil
		}
	}

	return "", fmt.Errorf("no external or internal IP found for node %s", node.Name)
}

// GetNodesByNodepool returns all nodes in a specific nodepool
func (c *Client) GetNodesByNodepool(nodepoolName string) ([]*corev1.Node, error) {
	// Create selector to filter by nodepool label
	selector := labels.Set{c.config.NodepoolLabelKey: nodepoolName}.AsSelector()
	
	// List nodes with the nodepool label
	nodeList, err := c.clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes in nodepool %s: %w", nodepoolName, err)
	}
	
	// Convert to pointer slice
	result := make([]*corev1.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		result = append(result, &nodeList.Items[i])
	}
	
	return result, nil
}

// LogEvent logs an event to the Kubernetes events API
func (c *Client) LogEvent(nodeName, eventType, reason, message string) error {
	// Create event
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", nodeName),
			Namespace:    "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Node",
			Name:      nodeName,
			Namespace: "",
			UID:       "",
		},
		Type:    eventType,
		Reason:  reason,
		Message: message,
		Source: corev1.EventSource{
			Component: "linode-database-allow-list",
		},
	}
	
	// Log to Kubernetes events API
	_, err := c.clientset.CoreV1().Events("default").Create(context.Background(), event, metav1.CreateOptions{})
	if err != nil {
		c.logger.Error("Failed to create Kubernetes event",
			zap.String("node", nodeName),
			zap.String("reason", reason),
			zap.Error(err),
		)
		return err
	}
	
	return nil
}

// getHostname returns the hostname of the current machine
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// setupWatchErrorHandler sets up a watch error handler for the given informer
func (c *Client) setupWatchErrorHandler(informer cache.SharedIndexInformer) {
	// Use recover to prevent panics from reflection
	defer func() {
		if r := recover(); r != nil {
			c.logger.Warn("Recovered from panic in setupWatchErrorHandler",
				zap.Any("panic", r),
			)
		}
	}()

	if informer == nil {
		c.logger.Warn("Cannot setup watch error handler: informer is nil")
		return
	}

	// Use safer reflection
	informerValue := reflect.ValueOf(informer)
	if !informerValue.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: informer value is invalid")
		return
	}
	
	// Check if we can access Elem()
	if informerValue.Kind() != reflect.Ptr || informerValue.IsNil() {
		c.logger.Warn("Cannot setup watch error handler: informer is not a valid pointer")
		return
	}
	
	informerElem := informerValue.Elem()
	if !informerElem.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: informer element is invalid")
		return
	}
	
	// Try to get controller field
	controllerField := informerElem.FieldByName("controller")
	if !controllerField.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: controller field not found")
		return
	}
	
	// Check if controller is a valid pointer
	if controllerField.Kind() != reflect.Ptr || controllerField.IsNil() {
		c.logger.Warn("Cannot setup watch error handler: controller is not a valid pointer")
		return
	}
	
	controllerElem := controllerField.Elem()
	if !controllerElem.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: controller element is invalid")
		return
	}
	
	// Try to get reflector field
	reflectorField := controllerElem.FieldByName("reflector")
	if !reflectorField.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: reflector field not found")
		return
	}
	
	// Check if reflector is a valid pointer
	if reflectorField.Kind() != reflect.Ptr || reflectorField.IsNil() {
		c.logger.Warn("Cannot setup watch error handler: reflector is not a valid pointer")
		return
	}
	
	reflectorElem := reflectorField.Elem()
	if !reflectorElem.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: reflector element is invalid")
		return
	}
	
	// Try to get watchErrorHandler field
	watcherField := reflectorElem.FieldByName("watchErrorHandler")
	if !watcherField.IsValid() {
		c.logger.Warn("Cannot setup watch error handler: watchErrorHandler field not found")
		return
	}
	
	// Check if watchErrorHandler is nil
	if watcherField.IsNil() {
		c.logger.Warn("Cannot setup watch error handler: watchErrorHandler is nil")
		return
	}
	
	c.logger.Debug("Setting up watch error handler")
	
	// The field exists and is not nil, so we can proceed
	try := func() bool {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Warn("Recovered from panic while setting up watch error handler",
					zap.Any("panic", r),
				)
			}
		}()
		
		originalHandler := watcherField.Interface().(cache.WatchErrorHandler)
		errorHandler := func(r *cache.Reflector, err error) {
			c.logger.Warn("Error watching Kubernetes resources, will retry",
				zap.String("reflector", r.Name()),
				zap.Error(err),
				zap.String("error_type", fmt.Sprintf("%T", err)),
			)
			// Log extra details for common watch errors
			if strings.Contains(err.Error(), "too old resource version") {
				c.logger.Warn("Watch expired - resource version too old. This is normal and will trigger a full relist.")
			} else if strings.Contains(err.Error(), "connection refused") {
				c.logger.Error("Connection to Kubernetes API server refused. Check network connectivity.")
			}
			originalHandler(r, err)
		}
		
		watcherField.Set(reflect.ValueOf(errorHandler))
		return true
	}
	
	if !try() {
		c.logger.Warn("Failed to set up watch error handler")
	}
}

// verifyWatchConsistency verifies the watch connection by listing nodes directly
// and comparing with what's in our cache
func (c *Client) verifyWatchConsistency(ctx context.Context) (bool, map[string]int) {
	// List nodes directly from the API
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		c.logger.Error("Failed to list nodes when verifying watch connection", zap.Error(err))
		return false, nil
	}
	
	// Count nodes by nodepool for verification
	nodepoolCounts := make(map[string]int)
	watchedNodepools := make(map[string]bool)
	
	// Build list of watched nodepools for easy lookup
	for _, np := range c.config.Nodepools {
		watchedNodepools[np.Name] = true
	}
	
	// Count nodes in each nodepool
	for _, node := range nodes.Items {
		nodepoolName := c.getNodepoolName(&node)
		if nodepoolName != "" {
			nodepoolCounts[nodepoolName]++
		}
	}
	
	// Log counts for watched nodepools
	for nodepool, count := range nodepoolCounts {
		if watchedNodepools[nodepool] {
			c.logger.Info("Current node count from direct API query", 
				zap.String("nodepool", nodepool), 
				zap.Int("count", count),
			)
		}
	}
	
	// Now verify against our informer cache by using GetNodesByNodepool
	consistent := true
	for nodepool := range watchedNodepools {
		cacheNodes, err := c.GetNodesByNodepool(nodepool)
		if err != nil {
			c.logger.Error("Failed to get nodes from cache when verifying", 
				zap.String("nodepool", nodepool),
				zap.Error(err),
			)
			consistent = false
			continue
		}
		
		apiCount := nodepoolCounts[nodepool]
		cacheCount := len(cacheNodes)
		
		if apiCount != cacheCount {
			consistent = false
			c.logger.Warn("Watch connection may have issues - node count mismatch",
				zap.String("nodepool", nodepool),
				zap.Int("api_count", apiCount),
				zap.Int("cache_count", cacheCount),
			)
			// Log the node names from both to help diagnose
			apiNodeNames := make([]string, 0, len(nodes.Items))
			for _, node := range nodes.Items {
				if c.getNodepoolName(&node) == nodepool {
					apiNodeNames = append(apiNodeNames, node.Name)
				}
			}
			
			cacheNodeNames := make([]string, 0, len(cacheNodes))
			for _, node := range cacheNodes {
				cacheNodeNames = append(cacheNodeNames, node.Name)
			}
			
			c.logger.Warn("Node list comparison",
				zap.String("nodepool", nodepool),
				zap.Strings("api_nodes", apiNodeNames),
				zap.Strings("cache_nodes", cacheNodeNames),
			)
		} else {
			c.logger.Info("Watch connection verification successful",
				zap.String("nodepool", nodepool),
				zap.Int("node_count", apiCount),
			)
		}
	}

	return consistent, nodepoolCounts
} 