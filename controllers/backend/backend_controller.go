package backend

import (
	"fmt"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// Provider interface method signatures
type Provider interface {
	Connect() error

	GetMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	CreateMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	EditMonitor(*lbv1.Monitor) (*lbv1.Monitor, error)
	DeleteMonitor(name string, url string, port int) (lbv1.Monitor, error)

	GetPool(name string) (string, error)
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
func CreateProvider(lbBackend *lbv1.LoadBalancerBackend) (*Provider, error) {

	// Get provider username and password
	userSecret := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: lbBackend.Spec.Provider.Creds},
			Key: "username"},
	}
	passwordSecret := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: lbBackend.Spec.Provider.Creds},
			Key: "password"},
	}

	//Treat secret errors here
	if userSecret == nil || passwordSecret == nil {
		return nil, fmt.Errorf("Provider secret credentials not found")
	}

	// Create backend provider based on backend type
	var provider Provider
	var err error
	switch lbBackend.Spec.Provider.Vendor {
	case "F5":
		provider, err = Create(*lbBackend, userSecret.String(), passwordSecret.String())

	default:
		err = fmt.Errorf("Provider not implemented")

	}

	if err != nil {
		return nil, err
	}
	return &provider, nil
}

//HandleMonitors manages the Monitor validation, update and creation
func HandleMonitors(p Provider, monitor lbv1.Monitor) (lbv1.Monitor, error) {
	// Check if monitor exists

	m, err := p.GetMonitor(&monitor)

	// Error getting monitor
	if err != nil {
		return lbv1.Monitor{}, err
	}

	// Monitor is not empty so update it's data if needed
	if m != nil {
		// Exists, so check to Update Monitor ports and parameters
		if monitor.Port != m.Port || monitor.Path != m.Path || monitor.MonitorType != m.MonitorType {
			m, err = p.EditMonitor(&monitor)
		}
		return *m, nil
	}

	// Create Monitor
	newmonitor, err := p.CreateMonitor(&monitor)
	if err != nil {
		return lbv1.Monitor{}, err
	}
	return *newmonitor, nil
}

//HandlePool manages the Pool validation, update and creation
func HandlePool(p Provider, name string, nodeIPs map[string]string, port int) (members []string, err error) {
	// Check if pool exists

	// if doesn't exist, create pool

	// Check pool members

	// Create pool members that do not exist

	// update pool adding new members and removing not used ones

	return nil, nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP(p Provider, name string, VIP string, pool string, port int) (vip string, err error) {
	// Check if VIP exists

	// if doesn't exist, create VIP

	// attach pool

	// update VIP ports and parameters

	return "", nil
}
