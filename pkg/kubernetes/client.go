package kubernetes

import (
	"context"
	"fmt"
	"os"
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
	// Create shared informer factory
	factory := informers.NewSharedInformerFactoryWithOptions(
		c.clientset,
		ResyncPeriod,
	)

	// Create node informer
	nodeInformer := factory.Core().V1().Nodes().Informer()

	// Set up event handlers
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
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
		DeleteFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
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
	factory.Start(c.stopCh)
	
	// Wait for cache sync
	if !cache.WaitForCacheSync(c.stopCh, nodeInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for node informer cache to sync")
	}
	
	// Block until stop channel is closed
	<-ctx.Done()
	return nil
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
	
	return node.Labels[NodePoolLabelKey]
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
	selector := labels.Set{NodePoolLabelKey: nodepoolName}.AsSelector()
	
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

// getHostname returns the hostname of the pod
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
} 