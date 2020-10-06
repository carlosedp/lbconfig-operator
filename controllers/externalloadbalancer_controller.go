package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"

	plog "log"

	"github.com/carlosedp/lbconfig-operator/controllers/backend"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// ExternalLoadBalancerReconciler reconciles a ExternalLoadBalancer object
type ExternalLoadBalancerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// LoadBalancerIPType defines the kind of IP that the operator fetches for the node
var LoadBalancerIPType corev1.NodeAddressType = "ExternalIP"

// ExternalLoadBalancerFinalizer is the finalizer object
const ExternalLoadBalancerFinalizer = "finalizer.lb.lbconfig.io"

func init() {
	// Disable backend logs using log module
	if _, present := os.LookupEnv("BACKEND_LOGS"); !present {
		plog.SetOutput(ioutil.Discard)
		plog.SetFlags(0)
	}
	// LoadBalancerIPType defines the kind of IP that the operator fetches for the node
	if _, KIND := os.LookupEnv("KIND"); KIND {
		LoadBalancerIPType = "InternalIP"
	}
}

// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=externalloadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=externalloadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=loadbalancerbackends,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lb.lbconfig.io,resources=loadbalancerbackends/status,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch

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
		return ctrl.Result{}, fmt.Errorf("Could not find LoadBalancerBackend %s", lb.Spec.Backend)
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
	// Get Nodes by role and label for infra router sharding or service exposure
	// ----------------------------------------
	if lb.Spec.Type == "" && lb.Spec.NodeLabels == nil {
		log.Error(err, "undefined loadbalancer type or no nodelabels defined")
		return ctrl.Result{}, err
	}

	var nodeList corev1.NodeList

	labels := computeLabels(lb)

	if err := r.List(ctx, &nodeList, client.MatchingLabels(labels)); err != nil {
		log.Error(err, "unable to list Nodes")
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Check if node is eligible by label and status
	// ----------------------------------------
	var nodes []lbv1.Node
	for _, n := range nodeList.Items {
		log.Info("Processing node", "node", n.Name, "labels", n.Labels)
		nodeAddrs := n.Status.Addresses
		var nodeReady bool
		for _, cond := range n.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				nodeReady = true
			}
		}
		for _, addr := range nodeAddrs {
			if addr.Type == LoadBalancerIPType && nodeReady {
				node := &lbv1.Node{
					Name:   n.Name,
					Host:   addr.Address,
					Labels: labels,
				}
				log.Info("Node matches", "node", node.Name, "labels", node.Labels, "ip", node.Host)
				nodes = append(nodes, *node)
			}
		}
	}

	// ----------------------------------------
	// Create Backend Provider
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
	monitor := lb.Spec.Monitor
	err = backend.HandleMonitors(log, provider, &monitor)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to handle ExternalLoadBalancer monitors: %v", err)
	}

	// ----------------------------------------
	// Handle IP Pools
	// ----------------------------------------
	var pools []lbv1.Pool
	for _, p := range lb.Spec.Ports {

		// Create the pool object
		var pool lbv1.Pool
		pool.Name = "Pool-" + lb.Name + "-" + strconv.Itoa(p)
		pool.Monitor = monitor.Name
		var poolMembers []lbv1.PoolMember
		for _, n := range nodes {
			poolMember := &lbv1.PoolMember{
				Node: n,
				Port: p,
			}
			poolMembers = append(poolMembers, *poolMember)
		}

		pool.Members = poolMembers

		err := backend.HandlePool(log, provider, &pool, &monitor)
		if err != nil {
			log.Error(err, "unable to handle ExternalLoadBalancer IP pool")
			return ctrl.Result{}, err
		}
		pools = append(pools, pool)
	}

	// ----------------------------------------
	// Handle VIPs
	// ----------------------------------------
	var vips []lbv1.VIP
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
		vips = append(vips, *newVIP)
	}

	// ----------------------------------------
	// Update ExternalLoadBalancer Status
	// ----------------------------------------
	_ = r.Get(ctx, req.NamespacedName, lb)
	lb.Status = lbv1.ExternalLoadBalancerStatus{
		VIPs:    vips,
		Monitor: monitor,
		Ports:   lb.Spec.Ports,
		Nodes:   nodes,
		Pools:   pools,
	}

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
			if err := r.finalizeLoadBalancer(log, provider, lb); err != nil {
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
		Watches(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				externalLoadBalancerList := &lbv1.ExternalLoadBalancerList{}
				client := mgr.GetClient()

				err := client.List(context.TODO(), externalLoadBalancerList)
				if err != nil {
					return []reconcile.Request{}
				}
				var reconcileRequests []reconcile.Request
				if node, ok := obj.Object.(*corev1.Node); ok {
					// Reconcile all ExternalLoadBalancers that match labels
					lbToReconcile := make(map[string]bool)
					for _, lb := range externalLoadBalancerList.Items {
						labels := computeLabels(&lb)
						if containsLabels(node.Labels, labels) {
							rec := reconcile.Request{
								NamespacedName: types.NamespacedName{
									Name:      lb.Name,
									Namespace: lb.Namespace,
								},
							}
							if _, ok := lbToReconcile[lb.Name]; !ok {
								lbToReconcile[lb.Name] = true
								reconcileRequests = append(reconcileRequests, rec)
							}
						}
					}
				}
				return reconcileRequests
			}),
		}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				if _, ok := e.ObjectNew.(*corev1.Node); ok {
					return hasNodeChanged(
						e.ObjectOld.(*corev1.Node),
						e.ObjectNew.(*corev1.Node))
				}
				return true
			},
		}).
		Complete(r)
}

func (r *ExternalLoadBalancerReconciler) finalizeLoadBalancer(reqLogger logr.Logger, p backend.Provider, lb *lbv1.ExternalLoadBalancer) error {
	err := backend.HandleCleanup(reqLogger, p, lb)
	if err != nil {
		reqLogger.Error(err, "error finalizing ExternalLoadBalancer")
		return err
	}
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

func hasNodeChanged(o *corev1.Node, n *corev1.Node) bool {
	var oldCond corev1.ConditionStatus
	var newCond corev1.ConditionStatus
	var oldIP string
	var newIP string

	for _, cond := range o.Status.Conditions {
		if cond.Type == "Ready" {
			oldCond = cond.Status
		}
	}
	for _, cond := range n.Status.Conditions {
		if cond.Type == "Ready" {
			newCond = cond.Status
		}
	}
	for _, addr := range o.Status.Addresses {
		if addr.Type == LoadBalancerIPType {
			oldIP = addr.Address
		}
	}
	for _, addr := range n.Status.Addresses {
		if addr.Type == LoadBalancerIPType {
			newIP = addr.Address
		}
	}

	if (oldCond == newCond) && (oldIP == newIP) && reflect.DeepEqual(o.Labels, n.Labels) {
		return false
	}
	return true
}

// Auxiliary functions

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func computeLabels(lb *lbv1.ExternalLoadBalancer) map[string]string {
	labels := make(map[string]string)
	if lb.Spec.Type != "" {
		labels["node-role.kubernetes.io/"+lb.Spec.Type] = ""
	}
	if lb.Spec.NodeLabels != nil {
		for k, v := range lb.Spec.NodeLabels {
			labels[k] = v
		}
	}
	return labels
}

func containsLabels(as, bs map[string]string) bool {
	labels := make(map[string]string)
	for k, v := range bs {
		if _, ok := as[k]; ok {
			labels[k] = v
		}
	}
	if reflect.DeepEqual(bs, labels) {
		return true
	}
	return false
}
