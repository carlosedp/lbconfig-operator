package dummy

import (
	"strconv"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/go-logr/logr"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// Provider is the object for the dummy provider implementing the Provider interface
type Provider struct {
	log           logr.Logger
	host          string
	hostport      int
	username      string
	password      string
	validatecerts bool
}

// Create creates a new Load Balancer backend provider
func Create(log logr.Logger, lbBackend lbv1.LoadBalancerBackend, username string, password string) (*Provider, error) {
	var p = &Provider{
		log:           log,
		host:          lbBackend.Spec.Provider.Host,
		hostport:      lbBackend.Spec.Provider.Port,
		validatecerts: *lbBackend.Spec.Provider.ValidateCerts,
		username:      username,
		password:      password,
	}
	log.WithValues("backend", lbBackend.Name, "provider", "dummy")
	err := p.Connect()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Connect creates a connection to the IP Load Balancer
func (p *Provider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.log.Info("Connect to dummy backend request", "host", host)
	return nil
}

// Close closes the connection to the IP Load Balancer
func (p *Provider) Close() error {
	p.log.Info("Close connection to dummy backend")
	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *Provider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	p.log.Info("Get dummy backend monitor objects")

	mon := &lbv1.Monitor{
		Name:        "MonitorName",
		MonitorType: "http",
		Path:        "/",
		Port:        1234,
	}
	p.log.Info("Get dummy backend monitor objects", "monitor", mon)
	return mon, nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *Provider) CreateMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {
	p.log.Info("Request to create a monitor in the dummy backend", "monitor", m)
	return m, nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *Provider) EditMonitor(m *lbv1.Monitor) error {
	p.log.Info("Request to edit a monitor in the dummy backend", "monitor", m)
	return nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *Provider) DeleteMonitor(m *lbv1.Monitor) error {
	p.log.Info("Request to delete a monitor in the dummy backend", "monitor", m)
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *Provider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	p.log.Info("Get dummy backend server pool")

	return nil, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *Provider) CreatePool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	p.log.Info("Creating Pool", "pool", pool.Name)
	return pool, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *Provider) EditPool(pool *lbv1.Pool) error {
	p.log.Info("Editing Pool", "pool", pool.Name)
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *Provider) DeletePool(pool *lbv1.Pool) error {
	p.log.Info("Deleting Pool", "pool", pool.Name)
	return nil
}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *Provider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *Provider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	p.log.Info("Editing pool member", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *Provider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Deleting pool member", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *Provider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	p.log.Info("Get dummy backend VIP")
	return nil, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *Provider) CreateVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	p.log.Info("Creating VIP", "vip", v.Name)
	return v, nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *Provider) EditVIP(v *lbv1.VIP) error {
	p.log.Info("Editing VIP", "vip", v.Name)
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *Provider) DeleteVIP(v *lbv1.VIP) error {
	p.log.Info("Deleting VIP", "vip", v.Name)
	return nil
}
