/*
MIT License

Copyright (c) 2022 Carlos Eduardo de Paula

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controllers

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	plog "log"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	controller "github.com/carlosedp/lbconfig-operator/controllers/backend/backend_controller"
	_ "github.com/carlosedp/lbconfig-operator/controllers/backend/backend_loader"
)

// ExternalLoadBalancerReconciler reconciles a ExternalLoadBalancer object
type ExternalLoadBalancerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Tracer name
const name = "github.com/carlosedp/lbconfig-operator"

// ExternalLoadBalancerFinalizer is the finalizer object
const ExternalLoadBalancerFinalizer = "lb.lbconfig.carlosedp.com/finalizer"

// Definition of Prometheus metrics
var (
	metric_externallb = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "externallb_total",
			Help: "Number of external load balancers configured",
		},
	)
	metric_externallb_nodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "externallb_nodes",
			Help: "Number of nodes for the load balancer instance",
		},
		[]string{"name", "namespace", "type", "vip", "port", "backend_vendor"},
	)
)

func init() {
	// Disable backend logs using log module
	if _, present := os.LookupEnv("BACKEND_LOGS"); !present {
		plog.SetOutput(io.Discard)
		plog.SetFlags(0)
	}
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(metric_externallb, metric_externallb_nodes)
}

// +kubebuilder:rbac:groups=lb.lbconfig.carlosedp.com,resources=externalloadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lb.lbconfig.carlosedp.com,resources=externalloadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch

// Reconcile our ExternalLoadBalancer object
func (r *ExternalLoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Each execution of the reconcile loop, create a new "root" span and context.
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "Reconcile")
	defer span.End()

	// Get our logger instance from context
	log := log.FromContext(ctx)
	log.Info("Starting reconcile loop for ExternalLoadBalancer")
	// ----------------------------------------
	// Get the LoadBalancer instance list to update metrics
	// ----------------------------------------
	lb_list := &lbv1.ExternalLoadBalancerList{}
	err := func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Get ExternalLoadBalancerList")
		defer span.End()
		return r.List(ctx, lb_list)
	}(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, fmt.Errorf("failed to list ExternalLoadBalancers: %v", err)
	}

	func(ctx context.Context) {
		_, span := otel.Tracer(name).Start(ctx, "metric.metric_externallb")
		defer span.End()
		lbnum := len(lb_list.Items)
		span.SetAttributes(attribute.Float64("metric.metric_externallb.lbnum", float64(lbnum)))
		metric_externallb.Set(float64(lbnum))
	}(ctx)

	// ----------------------------------------
	// Get the LoadBalancer instance
	// ----------------------------------------
	lb := &lbv1.ExternalLoadBalancer{}
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Get ExternalLoadBalancer")
		defer span.End()
		return r.Get(ctx, req.NamespacedName, lb)
	}(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("ExternalLoadBalancer resource not found. Ignoring since object must be deleted")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get ExternalLoadBalancer")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return ctrl.Result{}, err
	}
	span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("lb.provider", lb.Spec.Provider.Vendor))

	// ----------------------------------------
	// Set the Load Balancer backend
	// ----------------------------------------
	lbBackend := lb.Spec.Provider

	// Get backend secret
	credsSecret := &corev1.Secret{}
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Get Backend Secret")
		span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("lb.provider", lb.Spec.Provider.Vendor), attribute.String("lb.provider.secret", lbBackend.Creds))
		defer span.End()
		return r.Get(ctx, types.NamespacedName{Name: lbBackend.Creds, Namespace: lb.Namespace}, credsSecret)
	}(ctx)

	if err != nil {
		log.Error(err, "failed to get secret")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}
	span.SetAttributes(attribute.String("lb.provider.secret", credsSecret.Name))
	username := string(credsSecret.Data["username"])
	password := string(credsSecret.Data["password"])

	// ----------------------------------------
	// Get Nodes by role and label for infra router sharding or service exposure
	// ----------------------------------------
	if lb.Spec.Type == "" && lb.Spec.NodeLabels == nil {
		err = fmt.Errorf("undefined loadbalancer type or no nodelabels defined")
		return ctrl.Result{Requeue: false}, err
	}

	labels := func(ctx context.Context) map[string]string {
		_, span := otel.Tracer(name).Start(ctx, "Compute node labels")
		defer span.End()
		return computeLabels(*lb)
	}(ctx)

	var nodeList corev1.NodeList
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Get NodeList")
		defer span.End()
		return r.List(ctx, &nodeList, client.MatchingLabels(labels))
	}(ctx)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		log.Error(err, "unable to list Nodes")
		span.End()
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Check if node is eligible by label and status
	// ----------------------------------------
	var nodes []lbv1.Node
	for _, n := range nodeList.Items {
		log.Info("Processing node", "node", n.Name, "labels", n.Labels)
		for _, cond := range n.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				node := &lbv1.Node{
					Name:   n.Name,
					Host:   getNodeIP(&n),
					Labels: labels,
				}
				log.Info("Node matches", "node", node.Name, "labels", node.Labels, "ip", node.Host)
				nodes = append(nodes, *node)
			}
		}

	}
	// Set metric to the number of nodes found
	func(ctx context.Context) {
		_, span := otel.Tracer(name).Start(ctx, "metric.update.metric_externallb_nodes")
		defer span.End()
		span.SetAttributes(attribute.String("metric.metric_externallb_nodes.lbname", lb.Name))
		span.SetAttributes(attribute.Float64("metric.metric_externallb_nodes.nodes", float64(len(nodes))))

		ports := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(lb.Spec.Ports)), ","), "[]")
		metric_externallb_nodes.WithLabelValues(lb.Name, lb.Namespace, lb.Spec.Type, lb.Spec.Vip, ports, lb.Spec.Provider.Vendor).Set(float64(len(nodes)))
	}(ctx)

	// ----------------------------------------
	// Create Backend Provider
	// ----------------------------------------
	backend, err := controller.CreateBackend(ctx, &lbBackend, username, password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Connect to Backend Provider
	// ----------------------------------------
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Provider - Connect")
		defer span.End()
		return backend.Provider.Connect()
	}(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Handle Monitor
	// ----------------------------------------
	monitorName := "Monitor-" + lb.Name
	lb.Spec.Monitor.Name = monitorName
	monitor := lb.Spec.Monitor
	err = backend.HandleMonitors(ctx, &monitor)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, fmt.Errorf("unable to handle ExternalLoadBalancer monitors: %v", err)
	}

	// ----------------------------------------
	// Handle IP Pools
	// ----------------------------------------
	var pools []lbv1.Pool
	for _, p := range lb.Spec.Ports {
		// Create pool members based on nodes
		var poolMembers []lbv1.PoolMember
		for _, n := range nodes {
			poolMember := &lbv1.PoolMember{
				Node: n,
				Port: p,
			}
			poolMembers = append(poolMembers, *poolMember)
		}

		// Create the pool object
		pool := lbv1.Pool{
			Name:    "Pool-" + lb.Name + "-" + strconv.Itoa(p),
			Monitor: monitor.Name,
			Members: poolMembers,
		}

		err := backend.HandlePool(ctx, &pool, &monitor)
		if err != nil {
			log.Error(err, "unable to handle ExternalLoadBalancer IP pool")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return ctrl.Result{}, err
		}
		pools = append(pools, pool)
	}

	// ----------------------------------------
	// Handle VIPs
	// ----------------------------------------
	var vips []lbv1.VIP
	for _, p := range lb.Spec.Ports {
		vip := lbv1.VIP{
			Name: "VIP-" + lb.Name + "-" + strconv.Itoa(p),
			IP:   lb.Spec.Vip,
			Pool: "Pool-" + lb.Name + "-" + strconv.Itoa(p),
			Port: p,
		}

		err := backend.HandleVIP(ctx, &vip)
		if err != nil {
			log.Error(err, "unable to handle ExternalLoadBalancer VIP")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return ctrl.Result{}, err
		}
		vips = append(vips, vip)
	}

	// ----------------------------------------
	// Close Provider and save config if required.
	// Depends on provider implementation
	// ----------------------------------------
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Provider - Close")
		defer span.End()
		return backend.Provider.Close()
	}(ctx)
	if err != nil {
		log.Error(err, "unable to close the backend provider")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Update ExternalLoadBalancer Status
	// ----------------------------------------
	_ = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Get LoadBalancer for Status update")
		defer span.End()
		return r.Get(ctx, req.NamespacedName, lb)
	}(ctx)

	lb.Status = lbv1.ExternalLoadBalancerStatus{
		VIPs:     vips,
		Monitor:  monitor,
		Ports:    lb.Spec.Ports,
		Nodes:    nodes,
		Pools:    pools,
		Provider: lb.Spec.Provider,
		Labels:   labels,
		NumNodes: len(nodes),
	}

	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Update LoadBalancer Status")
		defer span.End()
		return r.Status().Update(ctx, lb)
	}(ctx)
	if err != nil {
		log.Error(err, "unable to update ExternalLoadBalancer status")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return ctrl.Result{}, err
	}

	// ----------------------------------------
	// Check if the ExternalLoadBalancer instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	// ----------------------------------------
	isLoadBalancerMarkedToBeDeleted := func(ctx context.Context) bool {
		_, span := otel.Tracer(name).Start(ctx, "GetDeletionTimestamp")
		defer span.End()
		return lb.GetDeletionTimestamp() != nil
	}(ctx)

	if isLoadBalancerMarkedToBeDeleted {

		finalizers := func(ctx context.Context) []string {
			_, span := otel.Tracer(name).Start(ctx, "GetFinalizers - Remove finalizer")
			defer span.End()
			return lb.GetFinalizers()
		}(ctx)

		if contains(finalizers, ExternalLoadBalancerFinalizer) {
			// Run finalization logic for ExternalLoadBalancerFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.

			err = func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "finalizeLoadBalancer")
				defer span.End()
				return r.finalizeLoadBalancer(ctx, backend, lb)
			}(ctx)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.End()
				return ctrl.Result{}, err
			}

			// Remove ExternalLoadBalancerFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			func(ctx context.Context) {
				_, span := otel.Tracer(name).Start(ctx, "RemoveFinalizer")
				defer span.End()
				controllerutil.RemoveFinalizer(lb, ExternalLoadBalancerFinalizer)
			}(ctx)

			err = func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Update LoadBalancer")
				defer span.End()
				return r.Update(ctx, lb)
			}(ctx)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.End()
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	finalizers := func(ctx context.Context) []string {
		_, span := otel.Tracer(name).Start(ctx, "GetFinalizers - Add Finalizer")
		defer span.End()
		return lb.GetFinalizers()
	}(ctx)

	if !contains(finalizers, ExternalLoadBalancerFinalizer) {
		err = func(ctx context.Context) error {
			_, span := otel.Tracer(name).Start(ctx, "addFinalizer")
			defer span.End()
			return r.addFinalizer(ctx, lb)
		}(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return ctrl.Result{}, err
		}
	}

	log.Info("End of reconcile loop for ExternalLoadBalancer")
	return ctrl.Result{}, nil
}

// SetupWithManager adds the reconciler in the Manager
func (r *ExternalLoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lbv1.ExternalLoadBalancer{}).
		// Watch node changes
		Watches(&source.Kind{Type: &corev1.Node{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			externalLoadBalancerList := &lbv1.ExternalLoadBalancerList{}
			client := mgr.GetClient()

			err := client.List(context.TODO(), externalLoadBalancerList)
			if err != nil {
				return []reconcile.Request{}
			}
			var reconcileRequests []reconcile.Request
			if node, ok := obj.(*corev1.Node); ok {
				// Reconcile all ExternalLoadBalancers that match labels
				lbToReconcile := make(map[string]bool)
				for _, lb := range externalLoadBalancerList.Items {
					labels := computeLabels(lb)
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
		).
		// Filter watched events to check only some fields on Node updates
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

func (r *ExternalLoadBalancerReconciler) finalizeLoadBalancer(ctx context.Context, backend *controller.BackendController, lb *lbv1.ExternalLoadBalancer) error {
	// Create a span to track the finalizer of this load balancer
	var span trace.Span
	_, span = otel.Tracer(name).Start(ctx, "finalizeLoadBalancer")
	defer span.End()

	reqLogger := log.FromContext(ctx)
	err := backend.HandleCleanup(ctx, lb)
	if err != nil {
		reqLogger.Error(err, "error finalizing ExternalLoadBalancer")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Delete metrics since the load balancer is gone
	ports := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(lb.Spec.Ports)), ","), "[]")
	metric_externallb_nodes.DeleteLabelValues(lb.Name, lb.Namespace, lb.Spec.Type, lb.Spec.Vip, ports, lb.Spec.Provider.Vendor)
	reqLogger.Info("Successfully finalized ExternalLoadBalancer")
	return nil
}

func (r *ExternalLoadBalancerReconciler) addFinalizer(ctx context.Context, m *lbv1.ExternalLoadBalancer) error {
	var span trace.Span
	_, span = otel.Tracer(name).Start(ctx, "finalizeLoadBalancer")
	defer span.End()

	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Adding Finalizer for the ExternalLoadBalancer")
	controllerutil.AddFinalizer(m, ExternalLoadBalancerFinalizer)

	// Update CR
	err := r.Update(context.TODO(), m)
	if err != nil {
		reqLogger.Error(err, "Failed to update ExternalLoadBalancer with finalizer")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
