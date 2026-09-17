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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// Tracer name
const name = "github.com/carlosedp/lbconfig-operator"

// Provider interface method signatures
type Provider interface {
	// Create a new backend provider
	Create(context.Context, lbv1.Provider, string, string) error
	// Connect initializes a connection to the backend provider
	Connect() error
	// Close closes the connection to the backend provider
	Close() error

	// GetMonitor returns a monitor if it exists
	GetMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	// CreateMonitor creates a new monitor
	CreateMonitor(*lbv1.Monitor) error
	// EditMonitor updates a monitor
	EditMonitor(*lbv1.Monitor) error
	// DeleteMonitor deletes a monitor
	DeleteMonitor(*lbv1.Monitor) error

	//	GetPool returns a pool if it exists
	GetPool(*lbv1.Pool) (*lbv1.Pool, error)
	// CreatePool creates a new pool
	CreatePool(*lbv1.Pool) error
	// EditPool updates a pool
	EditPool(*lbv1.Pool) error
	// DeletePool deletes a pool
	DeletePool(*lbv1.Pool) error
	// GetPoolMembers returns a pool members if it exists
	GetPoolMembers(*lbv1.Pool) (*lbv1.Pool, error)
	// CreatePoolMember returns a pool if it exists
	CreatePoolMember(*lbv1.PoolMember, *lbv1.Pool) error
	// EditPoolMember updates a pool member
	EditPoolMember(*lbv1.PoolMember, *lbv1.Pool, string) error
	// DisablePoolMember disables a pool member to prevent new connections while allowing existing connections to complete
	DisablePoolMember(*lbv1.PoolMember, *lbv1.Pool) error
	// DeletePoolMember deletes a pool member
	DeletePoolMember(*lbv1.PoolMember, *lbv1.Pool) error

	// GetVIP returns a virtual server if it exists
	GetVIP(*lbv1.VIP) (*lbv1.VIP, error)
	// CreateVIP creates a new virtual server
	CreateVIP(*lbv1.VIP) error
	// EditVIP updates a virtual server
	EditVIP(*lbv1.VIP) error
	// DeleteVIP deletes a virtual server
	DeleteVIP(*lbv1.VIP) error
}

// ExternalLoadBalancerReconciler reconciles a ExternalLoadBalancer object
type BackendController struct {
	log      logr.Logger
	Provider Provider
}

var providers = make(map[string]Provider)

func ListProviders() []string {
	p := make([]string, 0, len(providers))
	for k := range providers {
		p = append(p, k)
	}
	return p
}

func RegisterProvider(name string, provider Provider) error {
	name_slug := strings.ToLower(name)
	var ctx = context.Background()
	log := ctrllog.FromContext(ctx)
	if _, exists := providers[name_slug]; exists {
		return fmt.Errorf("provider already exists, provider '%s' tried to register twice", name)
	}
	log.Info("Registering provider", "provider", name)
	providers[name_slug] = provider
	return nil
}

func CreateBackend(ctx context.Context, lbBackend *lbv1.Provider, username string, password string) (*BackendController, error) {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "CreateBackend")
	defer span.End()
	backend := &BackendController{}
	backend.log = ctrllog.FromContext(ctx)
	name := strings.ToLower(lbBackend.Vendor)
	if provider, ok := providers[name]; ok {
		err := func(ctx context.Context) error {
			_, span := otel.Tracer(name).Start(ctx, "Provider - Create")
			defer span.End()
			return provider.Create(ctx, *lbBackend, username, password)
		}(ctx)

		if err != nil {
			return nil, err
		}
		backend.log.Info("Created backend", "provider", lbBackend.Vendor)
		backend.Provider = provider
		return backend, nil
	}
	return nil, fmt.Errorf("no such provider: %s. Available vendor providers are %s", name, ListProviders())
}

// HandleMonitors manages the Monitor validation, update and creation
func (b *BackendController) HandleMonitors(ctx context.Context, monitor *lbv1.Monitor) error {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "HandleMonitors")
	span.SetAttributes(attribute.String("monitor.name", monitor.Name))
	defer span.End()

	// Check if monitor exists
	m, err := func(ctx context.Context) (*lbv1.Monitor, error) {
		_, span := otel.Tracer(name).Start(ctx, "Provider - GetMonitor")
		span.SetAttributes(attribute.String("monitor.name", monitor.Name))
		defer span.End()
		return b.Provider.GetMonitor(monitor)
	}(ctx)

	// Error getting monitor
	if err != nil {
		return fmt.Errorf("error getting monitor: %s", err)
	}

	// Monitor is not empty so update it's data if needed
	if m != nil {
		// Exists, so check to Update Monitor ports and parameters
		b.log.Info("Monitor exists, check if needs update", "name", m.Name)
		span.SetAttributes(attribute.Bool("monitor.exists", true))
		if monitor.Port != m.Port || monitor.Path != m.Path || monitor.MonitorType != m.MonitorType {
			b.log.Info("Monitor requires update", "name", monitor.Name)
			b.log.Info("Need", "params", monitor)
			b.log.Info("Have", "params", m)
			span.SetAttributes(attribute.Bool("monitor.update", true))

			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - EditMonitor")
				span.SetAttributes(attribute.String("monitor.name", m.Name))
				defer span.End()
				return b.Provider.EditMonitor(monitor)
			}(ctx)
			if err != nil {
				return err
			}
			b.log.Info("Monitor updated successfully", "name", monitor.Name)
		} else {
			span.SetAttributes(attribute.Bool("monitor.update", false))
			b.log.Info("Monitor does not need update", "name", m.Name)
		}
		return nil
	}

	// Create Monitor
	b.log.Info("Monitor does not exist. Creating...", "name", monitor.Name)
	span.SetAttributes(attribute.Bool("monitor.exists", false))
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Provider - CreateMonitor")
		span.SetAttributes(attribute.String("monitor.name", monitor.Name))
		defer span.End()
		return b.Provider.CreateMonitor(monitor)
	}(ctx)
	if err != nil {
		return err
	}
	b.log.Info("Created monitor", "name", monitor.Name, "port", monitor.Port)
	return nil
}

// DefaultDrainTimeoutSeconds is the drain timeout used when draining is enabled without an explicit timeout
const DefaultDrainTimeoutSeconds = 30

// HandlePool manages the Pool validation, update and creation.
// drainingMembers holds the members currently being drained (persisted in the ExternalLoadBalancer status).
// It returns how long to wait until a draining member is due for removal (0 if no member is draining)
// and the updated draining members list.
func (b *BackendController) HandlePool(ctx context.Context, pool *lbv1.Pool, monitor *lbv1.Monitor, lb *lbv1.ExternalLoadBalancer, drainingMembers []lbv1.DrainingMember) (time.Duration, []lbv1.DrainingMember, error) {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "HandlePool")
	span.SetAttributes(attribute.String("pool.name", pool.Name))
	defer span.End()

	// Check if pool exists
	p, err := func(ctx context.Context) (*lbv1.Pool, error) {
		_, span := otel.Tracer(name).Start(ctx, "Provider - GetPool")
		span.SetAttributes(attribute.String("pool.name", pool.Name))
		defer span.End()
		return b.Provider.GetPool(pool)
	}(ctx)
	if err != nil {
		return 0, drainingMembers, err
	}

	// Pool is not empty so update it's data if needed
	if p != nil {
		span.SetAttributes(attribute.Bool("pool.exists", true))

		// Check if pool have members and update the object
		configuredPool, err := func(ctx context.Context) (*lbv1.Pool, error) {
			_, span := otel.Tracer(name).Start(ctx, "Provider - GetPoolMembers")
			span.SetAttributes(attribute.String("pool.name", p.Name))
			defer span.End()
			return b.Provider.GetPoolMembers(p)
		}(ctx)
		if err != nil {
			return 0, drainingMembers, err
		}

		// Exists, so check to Update pool parameters and members
		b.log.Info("Pool exists, check if needs update", "name", configuredPool.Name)
		var addMembers []lbv1.PoolMember
		var delMembers []lbv1.PoolMember

		// Check members that need to be added
		for _, m := range pool.Members {
			if !ContainsMember(configuredPool.Members, m) {
				// Add member to configuration
				addMembers = append(addMembers, m)
			}
		}
		// Check members that need to be removed
		for _, m := range configuredPool.Members {
			if !ContainsMember(pool.Members, m) {
				// Remove member from configuration
				delMembers = append(delMembers, m)
			}
		}

		// Stop tracking draining members that are no longer pending removal (eg. re-added during the drain)
		drainingMembers, err = b.releaseDrainingMembers(ctx, pool, configuredPool, delMembers, drainingMembers)
		if err != nil {
			return 0, drainingMembers, err
		}

		if pool.Monitor != configuredPool.Monitor {
			span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.Bool("pool.update", true))
			b.log.Info("Pool requires update", "name", pool.Name)
			b.log.Info("Need", "params", pool)
			b.log.Info("Have", "params", configuredPool)
			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - EditPool")
				span.SetAttributes(attribute.String("pool.name", pool.Name))
				defer span.End()
				return b.Provider.EditPool(pool)
			}(ctx)
			if err != nil {
				return 0, drainingMembers, err
			}
		}

		if addMembers != nil || delMembers != nil {
			span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.Bool("pool.members.update", true))
			b.log.Info("Pool members requires update", "name", pool.Name)
			b.log.Info("Need", "params", pool)
			b.log.Info("Have", "params", configuredPool)
			// Add members
			if addMembers != nil {
				b.log.Info("Add nodes", "nodes", addMembers)
				for _, m := range addMembers {
					err := func(ctx context.Context) error {
						_, span := otel.Tracer(name).Start(ctx, "Provider - CreatePoolMember")
						span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.String("pool.member", m.Node.Name))
						defer span.End()
						return b.Provider.CreatePoolMember(&m, pool)
					}(ctx)
					if err != nil {
						return 0, drainingMembers, err
					}
				}
			}

			// Remove members, draining them first if enabled
			if delMembers != nil {
				b.log.Info("Remove nodes", "nodes", delMembers)
				var requeueAfter time.Duration
				requeueAfter, drainingMembers, err = b.removePoolMembers(ctx, pool, delMembers, lb.Spec.Drain, drainingMembers)
				if err != nil {
					return 0, drainingMembers, err
				}
				if requeueAfter > 0 {
					b.log.Info("Pool members still draining", "name", pool.Name, "requeueAfter", requeueAfter.String())
					return requeueAfter, drainingMembers, nil
				}
			}

			b.log.Info("Pool updated successfully", "name", pool.Name)
			return 0, drainingMembers, nil
		}
		b.log.Info("Pool does not need update", "name", pool.Name)
		span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.Bool("pool.members.update", false))
		return 0, drainingMembers, nil
	}

	// Creating pool
	b.log.Info("Pool does not exist. Creating...", "name", pool.Name)
	span.SetAttributes(attribute.Bool("pool.exists", false))
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Provider - CreatePool")
		span.SetAttributes(attribute.String("pool.name", pool.Name))
		defer span.End()
		return b.Provider.CreatePool(pool)
	}(ctx)
	if err != nil {
		return 0, drainingMembers, err
	}
	// Adding members to pool
	b.log.Info("Created pool", "name", pool.Name)
	for _, m := range pool.Members {
		b.log.Info("Adding node to pool", "node", m, "pool", pool)
		err = func(ctx context.Context) error {
			_, span := otel.Tracer(name).Start(ctx, "Provider - CreatePoolMember")
			span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.String("pool.member", m.Node.Name))
			defer span.End()
			return b.Provider.CreatePoolMember(&m, pool)
		}(ctx)
		if err != nil {
			return 0, drainingMembers, err
		}
	}
	return 0, drainingMembers, nil
}

// releaseDrainingMembers stops tracking the pool draining members that are no longer pending removal.
// When a node comes back (eg. its label is re-applied) while its member is still disabled on the load balancer,
// the member is neither added nor removed, so it is re-enabled here. Members that are no longer configured on
// the load balancer are just dropped from the list since they are recreated (enabled) if desired again.
func (b *BackendController) releaseDrainingMembers(ctx context.Context, pool *lbv1.Pool, configuredPool *lbv1.Pool, delMembers []lbv1.PoolMember, drainingMembers []lbv1.DrainingMember) ([]lbv1.DrainingMember, error) {
	for _, dm := range drainingMembers {
		m := lbv1.PoolMember{Node: dm.Node, Port: dm.Port}
		if dm.PoolName != pool.Name || ContainsMember(delMembers, m) {
			continue
		}
		if ContainsMember(pool.Members, m) && ContainsMember(configuredPool.Members, m) {
			b.log.Info("Re-enabling member that was re-added during drain", "node", dm.Node.Name, "pool", pool.Name)
			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - EditPoolMember")
				span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.String("pool.member", dm.Node.Name))
				defer span.End()
				return b.Provider.EditPoolMember(&m, pool, "enable")
			}(ctx)
			if err != nil {
				return drainingMembers, fmt.Errorf("error re-enabling drained member %s: %v", dm.Node.Host, err)
			}
		}
		drainingMembers = RemoveDrainingMember(drainingMembers, &m, pool.Name)
	}
	return drainingMembers, nil
}

// removePoolMembers removes members from the pool. When draining is enabled, a member is first disabled so it stops
// receiving new connections while the existing ones complete, and is only deleted once the drain timeout elapsed.
// It returns how long to wait until the next draining member is due for removal (0 if no member is draining).
func (b *BackendController) removePoolMembers(ctx context.Context, pool *lbv1.Pool, delMembers []lbv1.PoolMember, drain *lbv1.DrainConfig, drainingMembers []lbv1.DrainingMember) (time.Duration, []lbv1.DrainingMember, error) {
	drainEnabled := drain != nil && drain.Enabled
	drainTimeout := DefaultDrainTimeoutSeconds * time.Second
	if drainEnabled && drain.TimeoutSeconds > 0 {
		drainTimeout = time.Duration(drain.TimeoutSeconds) * time.Second
	}

	var requeueAfter time.Duration
	for _, m := range delMembers {
		if drainEnabled {
			var remaining time.Duration
			if dm := IsMemberDraining(drainingMembers, &m, pool.Name); dm == nil {
				// Disable the member and start tracking its drain
				b.log.Info("Starting graceful drain for member", "node", m.Node.Name, "pool", pool.Name, "timeout", drainTimeout.String())
				err := func(ctx context.Context) error {
					_, span := otel.Tracer(name).Start(ctx, "Provider - DisablePoolMember")
					span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.String("pool.member", m.Node.Name))
					defer span.End()
					return b.Provider.DisablePoolMember(&m, pool)
				}(ctx)
				if err != nil {
					return 0, drainingMembers, fmt.Errorf("error disabling member %s for draining: %v", m.Node.Host, err)
				}
				// Status timestamps have second precision, round the start time up so the drain is never shortened
				startTime := metav1.NewTime(time.Now().Truncate(time.Second).Add(time.Second))
				drainingMembers = append(drainingMembers, lbv1.DrainingMember{
					PoolName:  pool.Name,
					Node:      m.Node,
					Port:      m.Port,
					StartTime: startTime,
				})
				remaining = drainTimeout + time.Until(startTime.Time)
			} else {
				remaining = drainTimeout - time.Since(dm.StartTime.Time)
			}

			if remaining > 0 {
				b.log.Info("Member draining", "node", m.Node.Name, "pool", pool.Name, "remaining", remaining.Round(time.Second).String())
				if requeueAfter == 0 || remaining < requeueAfter {
					requeueAfter = remaining
				}
				continue
			}
			b.log.Info("Drain timeout expired, deleting member", "node", m.Node.Name, "pool", pool.Name)
		}

		err := func(ctx context.Context) error {
			_, span := otel.Tracer(name).Start(ctx, "Provider - DeletePoolMember")
			span.SetAttributes(attribute.String("pool.name", pool.Name), attribute.String("pool.member", m.Node.Name))
			defer span.End()
			return b.Provider.DeletePoolMember(&m, pool)
		}(ctx)
		if err != nil {
			return 0, drainingMembers, err
		}
		drainingMembers = RemoveDrainingMember(drainingMembers, &m, pool.Name)
	}
	return requeueAfter, drainingMembers, nil
}

// HandleVIP manages the VIP validation, update and creation
func (b *BackendController) HandleVIP(ctx context.Context, v *lbv1.VIP) error {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "HandleVIP")
	span.SetAttributes(attribute.String("vip.name", v.Name))
	defer span.End()

	// Check if VIP exists
	vs, err := func(ctx context.Context) (*lbv1.VIP, error) {
		_, span := otel.Tracer(name).Start(ctx, "Provider - GetVIP")
		span.SetAttributes(attribute.String("vip.name", v.Name))
		defer span.End()
		return b.Provider.GetVIP(v)
	}(ctx)

	// Error getting VIP
	if err != nil {
		return err
	}

	// VIP is not empty so update it's data if needed
	if vs != nil {
		// Exists, so check to update VIP parameters and pool
		b.log.Info("VIP exists, check if needs update", "name", vs.Name)
		span.SetAttributes(attribute.Bool("vip.exists", true))

		if v.Port != vs.Port || v.IP != vs.IP || v.Pool != vs.Pool {
			b.log.Info("VIP requires update", "name", v.Name)
			b.log.Info("Need", "params", v)
			b.log.Info("Have", "params", vs)
			span.SetAttributes(attribute.Bool("vip.update", true))
			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - EditVIP")
				span.SetAttributes(attribute.String("vip.name", v.Name))
				defer span.End()
				return b.Provider.EditVIP(v)
			}(ctx)

			if err != nil {
				return err
			}
			b.log.Info("VIP updated successfully", "name", vs.Name)
		} else {
			b.log.Info("VIP does not need update", "name", vs.Name)
			span.SetAttributes(attribute.Bool("vip.update", false))
		}
		return nil
	}

	// Create VIP
	b.log.Info("VIP does not exist. Creating...", "name", v.Name)
	span.SetAttributes(attribute.Bool("vip.exists", false))
	err = func(ctx context.Context) error {
		_, span := otel.Tracer(name).Start(ctx, "Provider - CreateVIP")
		span.SetAttributes(attribute.String("vip.name", v.Name))
		defer span.End()
		return b.Provider.CreateVIP(v)
	}(ctx)
	if err != nil {
		return err
	}
	b.log.Info("Created VIP", "name", v.Name, "port", v.Port, "VIP", v.IP, "pool", v.Pool)
	return nil
}

// HandleCleanup removes all elements when ExternalLoadBalancer is deleted
func (b *BackendController) HandleCleanup(ctx context.Context, lb *lbv1.ExternalLoadBalancer) error {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "HandleCleanup")
	span.SetAttributes(attribute.String("lb.name", lb.Name))
	defer span.End()

	b.log.Info("Cleanup started", "ExternalLoadBalancer", lb.Name)

	// Delete VIP
	if len(lb.Status.VIPs) != 0 {
		for _, v := range lb.Status.VIPs {
			b.log.Info("Cleaning VIP", "VIP", v.Name)
			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - DeleteVIP")
				span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("vip.name", v.Name))
				defer span.End()
				return b.Provider.DeleteVIP(&v)
			}(ctx)
			if err != nil {
				return fmt.Errorf("error in VIP cleanup %s: %v", v.Name, err)
			}
		}
	}
	// Delete pool members
	if len(lb.Status.Pools) != 0 {
		for _, p := range lb.Status.Pools {
			for _, m := range p.Members {
				b.log.Info("Cleaning pool member", "pool", p.Name, "node", p.Name, "ip", m.Node.Host)
				err := func(ctx context.Context) error {
					_, span := otel.Tracer(name).Start(ctx, "Provider - DeletePoolMember")
					span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("pool.name", p.Name), attribute.String("pool.name", m.Node.Host))
					defer span.End()
					return b.Provider.DeletePoolMember(&m, &p)
				}(ctx)
				if err != nil {
					b.log.Info("Could not delete pool member", "host", m.Node.Host, "pool", p.Name, "error", err)
				}
			}
		}
	}

	// Delete Pool
	if len(lb.Status.Pools) != 0 {
		for _, pool := range lb.Status.Pools {
			b.log.Info("Cleaning pool", "pool", pool.Name)
			err := func(ctx context.Context) error {
				_, span := otel.Tracer(name).Start(ctx, "Provider - DeletePool")
				span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("pool.name", pool.Name))
				defer span.End()
				return b.Provider.DeletePool(&pool)
			}(ctx)
			if err != nil {
				return fmt.Errorf("error in pool cleanup %s: %v", pool.Name, err)
			}
		}
	}

	// Delete Monitor
	b.log.Info("Cleaning Monitor", "Monitor", lb.Status.Monitor)
	if lb.Status.Monitor != (lbv1.Monitor{}) {
		err := func(ctx context.Context) error {
			_, span := otel.Tracer(name).Start(ctx, "Provider - DeleteMonitor")
			span.SetAttributes(attribute.String("lb.name", lb.Name), attribute.String("monitor.name", lb.Status.Monitor.Name))
			defer span.End()
			return b.Provider.DeleteMonitor(&lb.Status.Monitor)
		}(ctx)
		if err != nil {
			return fmt.Errorf("error in monitor cleanup %s: %v", lb.Status.Monitor.Name, err)
		}
	}

	return nil
}

func ContainsMember(arr []lbv1.PoolMember, m lbv1.PoolMember) bool {
	for _, a := range arr {
		if a.Node.Host == m.Node.Host && a.Port == m.Port {
			return true
		}
	}
	return false
}

// IsMemberDraining checks if a pool member is currently in the draining state
func IsMemberDraining(drainingMembers []lbv1.DrainingMember, member *lbv1.PoolMember, poolName string) *lbv1.DrainingMember {
	for i := range drainingMembers {
		dm := &drainingMembers[i]
		if dm.PoolName == poolName &&
			dm.Node.Host == member.Node.Host &&
			dm.Port == member.Port {
			return dm
		}
	}
	return nil
}

// RemoveDrainingMember removes a member from the draining list
func RemoveDrainingMember(drainingMembers []lbv1.DrainingMember, member *lbv1.PoolMember, poolName string) []lbv1.DrainingMember {
	result := make([]lbv1.DrainingMember, 0, len(drainingMembers))
	for _, dm := range drainingMembers {
		if dm.PoolName != poolName || dm.Node.Host != member.Node.Host || dm.Port != member.Port {
			result = append(result, dm)
		}
	}
	return result
}
