package controllers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/carlosedp/lbconfig-operator/controllers/backend"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// ExternalLoadBalancerReconciler reconciles a ExternalLoadBalancer object
type ExternalLoadBalancerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=externalloadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=externalloadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=loadbalancerbackends,verbs=get;update;patch
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=loadbalancerbackends/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=secrets,verbs=get;list
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=nodes,verbs=get;list

// Reconcile our ExternalLoadBalancer object
func (r *ExternalLoadBalancerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("externalloadbalancer", req.NamespacedName)

	// Get the LoadBalancer instance
	lb := &lbv1.ExternalLoadBalancer{}
	err := r.Get(ctx, req.NamespacedName, lb)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("ExternalLoadBalancer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get ExternalLoadBalancer")
		return ctrl.Result{}, err
	}

	// Get Load Balancer backend
	lbBackend := &lbv1.LoadBalancerBackend{}
	err = r.Get(ctx, types.NamespacedName{Name: lb.Spec.Backend, Namespace: lb.Namespace}, lbBackend)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Could not find backend", "backend", lb.Spec.Backend)

		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get LoadBalancerBackend")
		return ctrl.Result{}, err
	}
	log.Info("Found backend", "backend", lbBackend.Name)

	// Get Nodes by role and label for infra router sharding
	var nodeList corev1.NodeList
	labels := make(map[string]string)
	labels["node-role.kubernetes.io/"+lb.Spec.Type] = ""
	if lb.Spec.ShardLabels != nil {
		for k, v := range lb.Spec.ShardLabels {
			labels[k] = v
		}
	}

	if err := r.List(ctx, &nodeList, client.MatchingLabels(labels)); err != nil {
		log.Error(err, "unable to list Nodes")
		return ctrl.Result{}, err
	}
	for _, node := range nodeList.Items {
		log.Info("Node matches", "node", node.Name, "labels", labels)
	}

	// Get the nodes external IPs
	nodeIPs := make(map[string]string)
	for _, n := range nodeList.Items {
		nodeAddrs := n.Status.Addresses
		for _, addr := range nodeAddrs {
			if addr.Type == "ExternalIP" {
				nodeIPs[n.Name] = addr.Address
			}
		}
	}

	// Handle Backend Provider
	// - Get Provider info
	// - Create connection?

	// Handle Monitors
	if err := backend.HandleMonitors(); err != nil {
		log.Error(err, "unable to handle ExternalLoadBalancer monitors")
		return ctrl.Result{}, err
	}

	// Handle IP Pools
	if err := backend.HandlePool(nodeIPs); err != nil {
		log.Error(err, "unable to handle ExternalLoadBalancer IP pool")
		return ctrl.Result{}, err
	}

	// Handle VIPs
	if err := backend.HandleVIP(); err != nil {
		log.Error(err, "unable to handle ExternalLoadBalancer VIP")
		return ctrl.Result{}, err
	}

	// Update ExternalLoadBalancer Status
	if err := r.Status().Update(ctx, lb); err != nil {
		log.Error(err, "unable to update ExternalLoadBalancer status")
		return ctrl.Result{}, err
	}

	// return ctrl.Result{Requeue: true}, nil // This is used to requeue the reconciliation
	return ctrl.Result{}, nil
}

// SetupWithManager adds the reconciler in the Manager
func (r *ExternalLoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lbv1.ExternalLoadBalancer{}).
		Complete(r)
}
