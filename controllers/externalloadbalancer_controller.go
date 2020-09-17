package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/carlosedp/lbconfig-operator/controllers/backend"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// ExternalLoadBalancerReconciler reconciles a ExternalLoadBalancer object
type ExternalLoadBalancerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

const ExternalLoadBalancerFinalizer = "finalizer.lb.lbconfig.io"

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

	// ----------------------------------------
	// Get the LoadBalancer instance
	// ----------------------------------------
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

	// ----------------------------------------
	// Get Load Balancer backend
	// ----------------------------------------
	lbBackend := &lbv1.LoadBalancerBackend{}
	err = r.Get(ctx, types.NamespacedName{Name: lb.Spec.Backend, Namespace: lb.Namespace}, lbBackend)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Could not find backend", "backend", lb.Spec.Backend)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "failed to get LoadBalancerBackend")
		return ctrl.Result{}, err
	}
	log.Info("Found backend", "backend", lbBackend.Name)

	credsSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: lbBackend.Spec.Provider.Creds, Namespace: lbBackend.Namespace}, credsSecret)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("provider credentials Secret not found %v", err)
	}
	username := string(credsSecret.Data["username"])
	password := string(credsSecret.Data["password"])

	// ----------------------------------------
	// Get Nodes by role and label for infra router sharding
	// ----------------------------------------
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

	// ----------------------------------------
	// Get the nodes external IPs
	// ----------------------------------------
	var members []lbv1.PoolMember
	for _, n := range nodeList.Items {
		nodeAddrs := n.Status.Addresses
		for _, addr := range nodeAddrs {
			if addr.Type == "ExternalIP" {
				m := &lbv1.PoolMember{
					Name:   n.Name,
					Host:   addr.Address,
					Labels: labels,
				}
				log.Info("Node matches", "node", n.Name, "labels", labels, "ip", addr.Address)
				members = append(members, *m)
			}
		}
	}

	// ----------------------------------------
	// Handle Backend Provider
	// - Get Provider info
	// - Create connection?
	// ----------------------------------------
	provider, err := backend.CreateProvider(log, lbBackend, username, password)
	if err != nil {
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Handle Monitor
	// ----------------------------------------
	monitorName := "Monitor-" + lb.Name
	lb.Spec.Monitor.Name = monitorName

	monitor, err := backend.HandleMonitors(log, provider, lb.Spec.Monitor)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to handle ExternalLoadBalancer monitors: %v", err)
	}
	lb.Status.Monitor = *monitor

	// ----------------------------------------
	// Handle IP Pools
	// ----------------------------------------
	var pools map[int]*lbv1.Pool
	for _, p := range lb.Spec.Ports {
		var pool lbv1.Pool
		pool.Name = "Pool-" + lb.Name + "-" + strconv.Itoa(p)
		pool.Monitor = monitor.Name
		pool.Members = members

		newPool, err := backend.HandlePool(log, provider, &pool, monitor)
		if err != nil {
			log.Error(err, "unable to handle ExternalLoadBalancer IP pool")
			return ctrl.Result{}, err
		}
		pools[p] = newPool
		lb.Status.PoolMembers = members
	}
	lb.Status.PoolMembers = members
	lb.Status.Ports = lb.Spec.Ports

	// ----------------------------------------
	// Handle VIPs
	// ----------------------------------------
	for _, p := range lb.Spec.Ports {
		var vip lbv1.VIP
		vip.Name = "VIP-" + lb.Name + "-" + strconv.Itoa(p)
		vip.Port = p
		vip.Pool = "Pool-" + lb.Name + "-" + strconv.Itoa(p)
		vip.IP = lb.Spec.Vip

		newVIP, err := backend.HandleVIP(log, provider, &vip)
		if err != nil {
			log.Error(err, "unable to handle ExternalLoadBalancer VIP")
			return ctrl.Result{}, err
		}
		lb.Status.VIP = *newVIP
	}

	// ----------------------------------------
	// Update ExternalLoadBalancer Status
	// ----------------------------------------
	if err := r.Status().Update(ctx, lb); err != nil {
		log.Error(err, "unable to update ExternalLoadBalancer status")
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Check if the ExternalLoadBalancer instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	// ----------------------------------------
	isLoadBalancerMarkedToBeDeleted := lb.GetDeletionTimestamp() != nil
	if isLoadBalancerMarkedToBeDeleted {
		if contains(lb.GetFinalizers(), ExternalLoadBalancerFinalizer) {
			// Run finalization logic for ExternalLoadBalancerFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeLoadBalancer(log, lb); err != nil {
				return ctrl.Result{}, err
			}

			// Remove ExternalLoadBalancerFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(lb, ExternalLoadBalancerFinalizer)
			err := r.Update(ctx, lb)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !contains(lb.GetFinalizers(), ExternalLoadBalancerFinalizer) {
		if err := r.addFinalizer(log, lb); err != nil {
			return ctrl.Result{}, err
		}
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

func (r *ExternalLoadBalancerReconciler) finalizeLoadBalancer(reqLogger logr.Logger, m *lbv1.ExternalLoadBalancer) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.
	reqLogger.Info("Successfully finalized ExternalLoadBalancer")
	return nil
}

func (r *ExternalLoadBalancerReconciler) addFinalizer(reqLogger logr.Logger, m *lbv1.ExternalLoadBalancer) error {
	reqLogger.Info("Adding Finalizer for the ExternalLoadBalancer")
	controllerutil.AddFinalizer(m, ExternalLoadBalancerFinalizer)

	// Update CR
	err := r.Update(context.TODO(), m)
	if err != nil {
		reqLogger.Error(err, "Failed to update ExternalLoadBalancer with finalizer")
		return err
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
