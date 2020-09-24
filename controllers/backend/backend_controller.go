package backend

import (
	"fmt"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/carlosedp/lbconfig-operator/controllers/backend/f5"
	"github.com/go-logr/logr"
)

// Provider interface method signatures
type Provider interface {
	Connect() error

	GetMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	CreateMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	EditMonitor(*lbv1.Monitor) error
	DeleteMonitor(*lbv1.Monitor) error

	GetPool(*lbv1.Pool) (*lbv1.Pool, error)
	CreatePool(*lbv1.Pool) (*lbv1.Pool, error)
	EditPool(*lbv1.Pool) error
	DeletePool(*lbv1.Pool) error

	CreatePoolMember(*lbv1.PoolMember, *lbv1.Pool) error
	EditPoolMember(*lbv1.PoolMember, *lbv1.Pool, string) error
	DeletePoolMember(*lbv1.PoolMember, *lbv1.Pool) error

	GetVIP(*lbv1.VIP) (*lbv1.VIP, error)
	CreateVIP(*lbv1.VIP) (*lbv1.VIP, error)
	EditVIP(*lbv1.VIP) error
	DeleteVIP(*lbv1.VIP) error
}

// CreateProvider creates a new backend provider
func CreateProvider(log logr.Logger, lbBackend *lbv1.LoadBalancerBackend, username string, password string) (Provider, error) {

	// Create backend provider based on backend type
	var provider Provider
	var err error
	switch lbBackend.Spec.Provider.Vendor {
	case "F5":
		provider, err = f5.Create(log, *lbBackend, username, password)

	default:
		err := fmt.Errorf("Provider not implemented")
		log.Error(err, "the configured provider is not  implemented", "provider", lbBackend.Spec.Provider.Vendor)
	}

	if err != nil {
		return nil, err
	}

	log.Info("Created backend", "provider", lbBackend.Spec.Provider.Vendor)
	return provider, nil
}

// HandleMonitors manages the Monitor validation, update and creation
func HandleMonitors(log logr.Logger, p Provider, monitor lbv1.Monitor) (*lbv1.Monitor, error) {
	// Check if monitor exists
	m, err := p.GetMonitor(&monitor)

	// Error getting monitor
	if err != nil {
		return nil, err
	}

	// Monitor is not empty so update it's data if needed
	if m != nil {
		// Exists, so check to Update Monitor ports and parameters
		log.Info("Monitor exists, check if needs update", "name", m.Name)
		if monitor.Port != m.Port || monitor.Path != m.Path || monitor.MonitorType != m.MonitorType {
			log.Info("Monitor requires update", "name", monitor.Name)
			log.Info("Need", "params", monitor)
			log.Info("Have", "params", m)
			err = p.EditMonitor(&monitor)
			if err != nil {
				return nil, err
			}
			log.Info("Monitor updated successfully", "name", monitor.Name)
		} else {
			log.Info("Monitor does not need update", "name", m.Name)
		}
		return &monitor, nil
	}

	// Create Monitor
	log.Info("Monitor does not exist. Creating...", "name", monitor.Name)
	newmonitor, err := p.CreateMonitor(&monitor)
	if err != nil {
		return nil, err
	}
	log.Info("Created monitor", "name", newmonitor.Name, "port", newmonitor.Port)
	return newmonitor, nil
}

// HandlePool manages the Pool validation, update and creation
func HandlePool(log logr.Logger, p Provider, pool *lbv1.Pool, monitor *lbv1.Monitor) error {
	// Check if pool exists
	configuredPool, err := p.GetPool(pool)

	// Error getting pool
	if err != nil {
		return err
	}

	// Pool is not empty so update it's data if needed
	if configuredPool != nil {
		// Exists, so check to Update pool parameters and members

		log.Info("Pool exists, check if needs update", "name", configuredPool.Name)
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

		if pool.Monitor != configuredPool.Monitor || addMembers != nil || delMembers != nil {
			log.Info("Pool requires update", "name", pool.Name)
			log.Info("Need", "params", pool)
			log.Info("Have", "params", configuredPool)
			err := p.EditPool(pool)
			if err != nil {
				return err
			}

			// Add members
			if addMembers != nil {
				log.Info("Add nodes", "nodes", addMembers)
				for _, m := range addMembers {
					err = p.CreatePoolMember(&m, pool)
					if err != nil {
						return err
					}
				}
			}

			// Remove members
			if delMembers != nil {
				log.Info("Remove nodes", "nodes", delMembers)
				for _, m := range addMembers {
					err = p.DeletePoolMember(&m, pool)
					if err != nil {
						return err
					}
				}
			}

			log.Info("Pool updated successfully", "name", pool.Name)
			return nil
		}
		log.Info("Pool does not need update", "name", pool.Name)
		return nil
	}

	// Creating pool
	log.Info("Pool does not exist. Creating...", "name", pool.Name)
	_, err = p.CreatePool(pool)
	if err != nil {
		return err
	}
	// Adding members to pool
	log.Info("Created pool", "name", pool.Name)
	for _, m := range pool.Members {
		log.Info("Adding node to pool", "node", m, "pool", pool)
		err = p.CreatePoolMember(&m, pool)
		if err != nil {
			return err
		}
	}
	return nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP(log logr.Logger, p Provider, v *lbv1.VIP) (vip *lbv1.VIP, err error) {
	// Check if VIP exists
	vs, err := p.GetVIP(v)
	// Error getting VIP
	if err != nil {
		return nil, err
	}

	// VIP is not empty so update it's data if needed
	if vs != nil {
		// Exists, so check to update VIP parameters and pool
		log.Info("VIP exists, check if needs update", "name", vs.Name)

		if v.Port != vs.Port || v.IP != vs.IP || v.Pool != vs.Pool {
			log.Info("VIP requires update", "name", v.Name)
			log.Info("Need", "params", v)
			log.Info("Have", "params", vs)
			err = p.EditVIP(v)
			if err != nil {
				return nil, err
			}
			log.Info("VIP updated successfully", "name", vs.Name)
		} else {
			log.Info("VIP does not need update", "name", vs.Name)
		}
		return vs, nil
	}

	// Create VIP
	log.Info("VIP does not exist. Creating...", "name", v.Name)
	newVIP, err := p.CreateVIP(v)
	if err != nil {
		return nil, err
	}
	log.Info("Created VIP", "name", newVIP.Name, "port", newVIP.Port, "VIP", newVIP.IP, "pool", newVIP.Pool)
	return newVIP, nil
}

// HandleCleanup removes all elements when ExternalLoadBalancer is deleted
func HandleCleanup(log logr.Logger, p Provider, lb *lbv1.ExternalLoadBalancer) error {
	log.Info("Cleanup started", "ExternalLoadBalancer", lb.Name)
	// Delete nodes

	// Delete Pool

	// Delete Monitor

	// Delete VIP
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
