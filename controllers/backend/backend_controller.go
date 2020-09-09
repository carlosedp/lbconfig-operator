package backend

import (
	"fmt"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// Provider interface method signatures
type Provider interface {
	Connect() error
	GetMonitor(name string, monitor lbv1.Monitor) (lbv1.Monitor, error)
	GetPool(name string) (string, error)
	GetVIP(name string) (string, error)

	CreateMonitor(name string, url string, port int) (string, error)
	EditMonitor(name string, url string, port int) (string, error)
	DeleteMonitor(name string, url string, port int) (string, error)

	CreateMember(node string, IP string) (string, error)
	EditPoolMember(name string, member string, port int, status string) (string, error)
	DeletePoolMember(name string, member string, port int, status string) (string, error)

	CreatePool(name string, monitor string, members []string, port int) (string, error)
	EditPool(name string, monitor string, members []string, port int) (string, error)
	DeletePool(name string, monitor string, members []string, port int) (string, error)

	CreateVIP(name string, VIP string, pool string, port int) (string, error)
	EditVIP(name string, VIP string, pool string, port int) (string, error)
	DeleteVIP(name string, VIP string, pool string, port int) (string, error)
}

// CreateProvider creates a new backend provider
func CreateProvider(lbBackend *lbv1.LoadBalancerBackend) (Provider, error) {

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

	var provider Provider
	// Create backend provider based on backend type
	switch lbBackend.Spec.Provider.Vendor {
	case "F5":
		provider = F5Provider{
			host:          lbBackend.Spec.Provider.Host,
			hostport:      lbBackend.Spec.Provider.Port,
			partition:     "Common",
			validatecerts: false,
			username:      userSecret.String(),
			password:      passwordSecret.String(),
		}
		provider.Connect()

	default:
		return nil, fmt.Errorf("Provider not implemented")

	}

	return provider, nil
}

//HandleMonitors manages the Monitor validation, update and creation
func HandleMonitors(p Provider, name string, monitor lbv1.Monitor) (lbv1.Monitor, error) {
	// Check if monitor exists

	_, err := p.GetMonitor(name, monitor)

	if err != nil {
		return lbv1.Monitor{}, nil

	}
	// Create Monitor

	// or Update Monitor ports and parameters
	return lbv1.Monitor{}, nil
}

//HandlePool manages the Pool validation, update and creation
func HandlePool(name string, nodeIPs map[string]string, port int) (members []string, err error) {
	// Check if pool exists

	// if doesn't exist, create pool

	// Check pool members

	// Create pool members that do not exist

	// update pool adding new members and removing not used ones

	return nil, nil
}

//HandleVIP manages the VIP validation, update and creation
func HandleVIP(name string, VIP string, pool string, port int) (vip string, err error) {
	// Check if VIP exists

	// if doesn't exist, create VIP

	// attach pool

	// update VIP ports and parameters

	return "", nil
}

func createMember(members map[string]string) error {
	for k, v := range members {
		nodeName := k
		IP := v
		_, _ = nodeName, IP
	}
	return nil
}
