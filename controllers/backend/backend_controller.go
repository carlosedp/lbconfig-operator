package backend

import (
	"fmt"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/go-logr/logr"
)

// Provider interface method signatures
type Provider interface {
	Connect() error

	GetMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	CreateMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	EditMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	DeleteMonitor(m *lbv1.Monitor) error

	GetPool(pool *lbv1.Pool) (*lbv1.Pool, error)
	CreatePool(name string, monitor string, members []string, port int) (string, error)
	EditPool(name string, monitor string, members []string, port int) (string, error)
	DeletePool(name string, monitor string, members []string, port int) (string, error)

	CreateMember(node string, IP string) (string, error)
	EditPoolMember(name string, member string, port int, status string) (string, error)
	DeletePoolMember(name string, member string, port int, status string) (string, error)

	GetVIP(name string) (string, error)
	CreateVIP(name string, VIP string, pool string, port int) (string, error)
	EditVIP(name string, VIP string, pool string, port int) (string, error)
	DeleteVIP(name string, VIP string, pool string, port int) (string, error)
}

// CreateProvider creates a new backend provider
func CreateProvider(log logr.Logger, lbBackend *lbv1.LoadBalancerBackend, username string, password string) (Provider, error) {

	// Create backend provider based on backend type
	var provider Provider
	var err error
	switch lbBackend.Spec.Provider.Vendor {
	case "F5":
		provider, err = Create(*lbBackend, username, password)

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
			m, err = p.EditMonitor(&monitor)
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
func HandlePool(log logr.Logger, p Provider, pool *lbv1.Pool, monitor *lbv1.Monitor) (*lbv1.Pool, error) {
	// Check if pool exists

	// pool exists, update if necessary

	//// Check pool members

	//// Create pool members that do not exist

	//// Update pool adding new members and removing not used ones

	// if pool doesn't exist, create

	return nil, nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP(log logr.Logger, p Provider, VIP *lbv1.VIP) (vip *lbv1.VIP, err error) {
	// Check if VIP exists

	// VIP exists, update if necessary

	//// attach pool

	//// update VIP ports and parameters

	// if VIP doesn't exist, create

	return nil, nil
}
