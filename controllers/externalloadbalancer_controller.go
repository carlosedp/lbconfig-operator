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

	// log.Info("ExternalLoadBalancer", "name", lb.Name, "backend", lb.Spec.Backend)
	backend := &lbv1.LoadBalancerBackend{}
	err = r.Get(ctx, types.NamespacedName{Name: lb.Spec.Backend, Namespace: lb.Namespace}, backend)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Could not find backend", "backend", lb.Spec.Backend)

		// return ctrl.Result{}, nil // <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<< REVERT
	} else if err != nil {
		log.Error(err, "Failed to get LoadBalancerBackend")
		return ctrl.Result{}, err
	}
	log.Info("Found backend", "backend", backend.Name)

	// Get Nodes by role and label for infra router sharding
	var nodeList corev1.NodeList

	labels := make(map[string]string)
	labels["node-role.kubernetes.io/"+lb.Spec.Type] = ""
	if lb.Spec.Shard != "" {

		labels[lb.Spec.Shard] = ""
	}

	if err := r.List(ctx, &nodeList, client.MatchingLabels(labels)); err != nil {
		log.Error(err, "unable to list Nodes")
		return ctrl.Result{}, err
	}
	for _, node := range nodeList.Items {
		log.Info("Node matches", "node", node.Name, "role", labels)
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
