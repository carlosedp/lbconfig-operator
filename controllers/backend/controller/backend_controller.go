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

	"github.com/go-logr/logr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

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
	CreateMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	// EditMonitor updates a monitor
	EditMonitor(*lbv1.Monitor) error
	// DeleteMonitor deletes a monitor
	DeleteMonitor(*lbv1.Monitor) error

	//	GetPool returns a pool if it exists
	GetPool(*lbv1.Pool) (*lbv1.Pool, error)
	// CreatePool creates a new pool
	CreatePool(*lbv1.Pool) (*lbv1.Pool, error)
	// EditPool updates a pool
	EditPool(*lbv1.Pool) error
	// DeletePool deletes a pool
	DeletePool(*lbv1.Pool) error
	// GetPoolMembers returns a pool members if it exists
	CreatePoolMember(*lbv1.PoolMember, *lbv1.Pool) error
	// EditPoolMember updates a pool member
	EditPoolMember(*lbv1.PoolMember, *lbv1.Pool, string) error
	// DeletePoolMember deletes a pool member
	DeletePoolMember(*lbv1.PoolMember, *lbv1.Pool) error

	// GetVIP returns a virtual server if it exists
	GetVIP(*lbv1.VIP) (*lbv1.VIP, error)
	// CreateVIP creates a new virtual server
	CreateVIP(*lbv1.VIP) (*lbv1.VIP, error)
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
	var p []string
	for k := range providers {
		p = append(p, k)
	}
	return p
}

func RegisterProvider(name string, provider Provider) {
	var ctx = context.Background()
	log := ctrllog.FromContext(ctx)
	log.WithValues("backend_controller", "RegisterProvider")
	if _, exists := providers[name]; exists {
		log.Error(fmt.Errorf("provider already exists"), "Provider '%s' tried to register twice", name)
		return
	}
	log.Info("Registering provider", "provider", name)
	providers[name] = provider
}

func CreateBackend(ctx context.Context, lbBackend *lbv1.Provider, username string, password string) (*BackendController, error) {
	backend := &BackendController{}
	backend.log = ctrllog.FromContext(ctx)
	backend.log.WithValues("backend_controller", "RegisterProvider")
	name := strings.ToLower(lbBackend.Vendor)
	if provider, ok := providers[name]; ok {
		if err := provider.Create(ctx, *lbBackend, username, password); err != nil {
			return nil, err
		}
		backend.log.Info("Created backend", "provider", lbBackend.Vendor)
		backend.Provider = provider
		return backend, nil
	}
	return nil, fmt.Errorf("no such provider: %s", name)
}

// HandleMonitors manages the Monitor validation, update and creation
func (b *BackendController) HandleMonitors(monitor *lbv1.Monitor) error {
	// Check if monitor exists
	m, err := b.Provider.GetMonitor(monitor)

	// Error getting monitor
	if err != nil {
		return err
	}

	// Monitor is not empty so update it's data if needed
	if m != nil {
		// Exists, so check to Update Monitor ports and parameters
		b.log.Info("Monitor exists, check if needs update", "name", m.Name)
		if monitor.Port != m.Port || monitor.Path != m.Path || monitor.MonitorType != m.MonitorType {
			b.log.Info("Monitor requires update", "name", monitor.Name)
			b.log.Info("Need", "params", monitor)
			b.log.Info("Have", "params", m)
			err = b.Provider.EditMonitor(monitor)
			if err != nil {
				return err
			}
			b.log.Info("Monitor updated successfully", "name", monitor.Name)
		} else {
			b.log.Info("Monitor does not need update", "name", m.Name)
		}
		return nil
	}

	// Create Monitor
	b.log.Info("Monitor does not exist. Creating...", "name", monitor.Name)
	_, err = b.Provider.CreateMonitor(monitor)
	if err != nil {
		return err
	}
	b.log.Info("Created monitor", "name", monitor.Name, "port", monitor.Port)
	return nil
}

// HandlePool manages the Pool validation, update and creation
func (b *BackendController) HandlePool(pool *lbv1.Pool, monitor *lbv1.Monitor) error {
	// Check if pool exists
	configuredPool, err := b.Provider.GetPool(pool)

	// Error getting pool
	if err != nil {
		return err
	}

	// Pool is not empty so update it's data if needed
	if configuredPool != nil {
		// Exists, so check to Update pool parameters and members

		b.log.Info("Pool exists, check if needs update", "name", configuredPool.Name)
		var addMembers []lbv1.PoolMember
		var delMembers []lbv1.PoolMember

		// Check members that need to be added
		for _, m := range pool.Members {
			if !containsMember(configuredPool.Members, m) {
				// Add member to configuration
				addMembers = append(addMembers, m)
			}
		}
		// Check members that need to be removed
		for _, m := range configuredPool.Members {
			if !containsMember(pool.Members, m) {
				// Remove member from configuration
				delMembers = append(delMembers, m)
			}
		}

		if pool.Monitor != configuredPool.Monitor {
			b.log.Info("Pool requires update", "name", pool.Name)
			b.log.Info("Need", "params", pool)
			b.log.Info("Have", "params", configuredPool)
			err := b.Provider.EditPool(pool)
			if err != nil {
				return err
			}
		}

		if addMembers != nil || delMembers != nil {
			b.log.Info("Pool members requires update", "name", pool.Name)
			b.log.Info("Need", "params", pool)
			b.log.Info("Have", "params", configuredPool)
			// Add members
			if addMembers != nil {
				b.log.Info("Add nodes", "nodes", addMembers)
				for _, m := range addMembers {
					err = b.Provider.CreatePoolMember(&m, pool)
					if err != nil {
						return err
					}
				}
			}
			// Remove members
			if delMembers != nil {
				b.log.Info("Remove nodes", "nodes", delMembers)
				for _, m := range delMembers {
					err = b.Provider.DeletePoolMember(&m, pool)
					if err != nil {
						return err
					}
				}
			}

			b.log.Info("Pool updated successfully", "name", pool.Name)
			return nil
		}
		b.log.Info("Pool does not need update", "name", pool.Name)
		return nil
	}

	// Creating pool
	b.log.Info("Pool does not exist. Creating...", "name", pool.Name)
	_, err = b.Provider.CreatePool(pool)
	if err != nil {
		return err
	}
	// Adding members to pool
	b.log.Info("Created pool", "name", pool.Name)
	for _, m := range pool.Members {
		b.log.Info("Adding node to pool", "node", m, "pool", pool)
		err = b.Provider.CreatePoolMember(&m, pool)
		if err != nil {
			return err
		}
	}
	return nil
}

//HandleVIP manages the VIP validation, update and creation
func (b *BackendController) HandleVIP(v *lbv1.VIP) (vip *lbv1.VIP, err error) {
	// Check if VIP exists
	vs, err := b.Provider.GetVIP(v)
	// Error getting VIP
	if err != nil {
		return nil, err
	}

	// VIP is not empty so update it's data if needed
	if vs != nil {
		// Exists, so check to update VIP parameters and pool
		b.log.Info("VIP exists, check if needs update", "name", vs.Name)

		if v.Port != vs.Port || v.IP != vs.IP || v.Pool != vs.Pool {
			b.log.Info("VIP requires update", "name", v.Name)
			b.log.Info("Need", "params", v)
			b.log.Info("Have", "params", vs)
			err = b.Provider.EditVIP(v)
			if err != nil {
				return nil, err
			}
			b.log.Info("VIP updated successfully", "name", vs.Name)
		} else {
			b.log.Info("VIP does not need update", "name", vs.Name)
		}
		return vs, nil
	}

	// Create VIP
	b.log.Info("VIP does not exist. Creating...", "name", v.Name)
	newVIP, err := b.Provider.CreateVIP(v)
	if err != nil {
		return nil, err
	}
	b.log.Info("Created VIP", "name", newVIP.Name, "port", newVIP.Port, "VIP", newVIP.IP, "pool", newVIP.Pool)
	return newVIP, nil
}

// HandleCleanup removes all elements when ExternalLoadBalancer is deleted
func (b *BackendController) HandleCleanup(lb *lbv1.ExternalLoadBalancer) error {
	b.log.Info("Cleanup started", "ExternalLoadBalancer", lb.Name)

	// Delete VIP
	if len(lb.Status.VIPs) != 0 {
		for _, v := range lb.Status.VIPs {
			b.log.Info("Cleaning VIP", "VIP", v.Name)
			err := b.Provider.DeleteVIP(&v)
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
				err := b.Provider.DeletePoolMember(&m, &p)
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
			err := b.Provider.DeletePool(&pool)
			if err != nil {
				return fmt.Errorf("error in pool cleanup %s: %v", pool.Name, err)
			}
		}
	}

	// Delete Monitor
	b.log.Info("Cleaning Monitor", "Monitor", lb.Status.Monitor)
	if lb.Status.Monitor != (lbv1.Monitor{}) {
		err := b.Provider.DeleteMonitor(&lb.Status.Monitor)
		if err != nil {
			return fmt.Errorf("error in Monitor cleanup %s: %v", lb.Status.Monitor.Name, err)
		}
	}

	return nil
}

func containsMember(arr []lbv1.PoolMember, m lbv1.PoolMember) bool {
	for _, a := range arr {
		if a.Node.Host == m.Node.Host && a.Port == m.Port {
			return true
		}
	}
	return false
}
