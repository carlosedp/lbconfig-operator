package f5

import (
	"fmt"
	"strconv"
	"strings"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/scottdware/go-bigip"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

const partition = "/Common/"

// Provider is the object for the F5 Big IP Provider implementing the Provider interface
type Provider struct {
	log           logr.Logger
	f5            *bigip.BigIP
	host          string
	hostport      int
	username      string
	password      string
	partition     string
	validatecerts bool
}

// Create creates a new Load Balancer backend provider
func Create(log logr.Logger, lbBackend lbv1.LoadBalancerBackend, username string, password string) (*Provider, error) {
	var p = &Provider{
		log:           log,
		host:          lbBackend.Spec.Provider.Host,
		hostport:      lbBackend.Spec.Provider.Port,
		partition:     lbBackend.Spec.Provider.Partition,
		validatecerts: *lbBackend.Spec.Provider.ValidateCerts,
		username:      username,
		password:      password,
	}
	log = log.WithValues("backend", lbBackend.Name, "provider", "F5")
	err := p.Connect()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Connect creates a connection to the IP Load Balancer
func (p *Provider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.f5 = bigip.NewSession(host, p.username, p.password, nil)

	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *Provider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	m, err := p.f5.GetMonitor(monitor.Name, monitor.MonitorType)
	if err != nil {
		return nil, fmt.Errorf("error getting F5 Monitor %s: %v", monitor.Name, err)
	}

	// Return in case monitor does not exist
	if m == nil {
		return nil, nil
	}

	// Return monitor details in case it exists
	s := strings.Split(m.Destination, ".")[1]
	port, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("error converting F5 monitor port: %v", err)
	}

	parent := strings.Split(m.ParentMonitor, "/")[2]
	mon := &lbv1.Monitor{
		Name:        m.Name,
		MonitorType: parent,
		Path:        strings.TrimLeft(m.SendString, "GET "),
		Port:        port,
	}

	return mon, nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *Provider) CreateMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {

	config := &bigip.Monitor{
		Name:          m.Name,
		ParentMonitor: partition + m.MonitorType,
		Interval:      5,
		Timeout:       16,
		SendString:    "GET " + m.Path,
		ReceiveString: "",
	}

	if m.Port != 0 {
		destination := "*." + strconv.Itoa(m.Port)
		config.Destination = destination
	}
	err := p.f5.AddMonitor(config, m.MonitorType)
	if err != nil {
		return nil, fmt.Errorf("error creating F5 monitor %s: %v", m.Name, err)
	}

	return m, nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *Provider) EditMonitor(m *lbv1.Monitor) error {
	config := &bigip.Monitor{
		Name:          m.Name,
		ParentMonitor: partition + m.MonitorType,
		Interval:      5,
		Timeout:       16,
		SendString:    "GET " + m.Path,
		ReceiveString: "",
	}

	// Cannot update monitor port.
	// TODO: Return error to be treated by the controller
	// if m.Port != 0 {
	// 	destination := "*." + strconv.Itoa(m.Port)
	// 	config.Destination = destination
	// }
	err := p.f5.PatchMonitor(m.Name, m.MonitorType, config)
	if err != nil {
		return fmt.Errorf("error patching F5 monitor  %s: %v", m.Name, err)
	}
	return nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *Provider) DeleteMonitor(m *lbv1.Monitor) error {
	err := p.f5.DeleteMonitor(m.Name, m.MonitorType)
	if err != nil {
		return fmt.Errorf("error deleting F5 monitor %s: %v", m.Name, err)
	}
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *Provider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {

	newPool, err := p.f5.GetPool(pool.Name)
	if err != nil {
		return nil, fmt.Errorf("error getting F5 pool: %v", err)
	}

	// Return in case pool does not exist
	if newPool == nil {
		p.log.Info("Pool does not exist")
		return nil, nil
	}

	// Get pool members
	var members []lbv1.PoolMember
	poolMembers, _ := p.f5.PoolMembers(pool.Name)
	for _, member := range poolMembers.PoolMembers {
		ip := strings.Split(member.Address, ":")[0]
		port, _ := strconv.Atoi(strings.Split(member.FullPath, ":")[1])
		node := &lbv1.Node{
			Name: member.Name,
			Host: ip,
		}
		mem := &lbv1.PoolMember{
			Node: *node,
			Port: port,
		}
		members = append(members, *mem)
	}

	retPool := &lbv1.Pool{
		Name:    newPool.Name,
		Monitor: strings.Trim(newPool.Monitor, partition),
		Members: members,
	}

	return retPool, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *Provider) CreatePool(pool *lbv1.Pool) (*lbv1.Pool, error) {

	// Create Pool
	err := p.f5.CreatePool(pool.Name)
	if err != nil {
		return nil, fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	// Add monitor to Pool
	err = p.f5.AddMonitorToPool(pool.Monitor, pool.Name)
	if err != nil {
		return nil, fmt.Errorf("error adding monitor %s to pool %s: %v", pool.Monitor, pool.Name, err)
	}
	return pool, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *Provider) EditPool(pool *lbv1.Pool) error {
	newPool := &bigip.Pool{
		Name:    pool.Name,
		Monitor: pool.Monitor,
	}

	err := p.f5.ModifyPool(pool.Name, newPool)
	if err != nil {
		return fmt.Errorf("error editing pool %s: %v", pool.Name, err)
	}
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *Provider) DeletePool(pool *lbv1.Pool) error {
	err := p.f5.DeletePool(pool.Name)
	if err != nil {
		return fmt.Errorf("error deleting pool %s: %v", pool.Name, err)
	}
	return nil
}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *Provider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)
	config := bigip.Node{
		Name:    m.Node.Host + ":" + strconv.Itoa(m.Port),
		Address: m.Node.Host,
	}
	// Query node by IP
	n, _ := p.f5.GetNode(m.Node.Host)
	if n != nil {
		p.f5.ModifyNode(m.Node.Host, &config)
	} else {
		err := p.f5.CreateNode(m.Node.Host, m.Node.Host)
		if err != nil {
			return fmt.Errorf("error creating node %s: %v", m.Node.Host, err)
		}
	}

	err := p.f5.AddPoolMember(pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port))
	if err != nil {
		return fmt.Errorf("error adding member %s to pool %s: %v", m.Node.Host, pool.Name, err)
	}

	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *Provider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	err := p.f5.PoolMemberStatus(pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port), status)
	if err != nil {
		return fmt.Errorf("error editing member %s in pool %s: %v", m.Node.Host, pool.Name, err)
	}
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *Provider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	// First delete member from pool
	err := p.f5.DeletePoolMember(partition+pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port))
	if err != nil {
		return fmt.Errorf("error removing member %s from pool %s: %v", m.Node.Host, pool.Name, err)
	}
	// Then delete node (Do not delete node since it could be in use on another LB)
	// err = p.f5.DeleteNode(partition + m.Node.Host)
	// if err != nil {
	// 	return fmt.Errorf("error deleting member %s: %v", m.Node.Host, err)
	// }
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *Provider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	vs, err := p.f5.GetVirtualServer(v.Name)
	if err != nil {
		return nil, fmt.Errorf("error getting F5 virtualserver %s: %v", v.Name, err)
	}

	// Return in case VIP does not exist
	if vs == nil {
		return nil, nil
	}

	// Return VIP details in case it exists
	s := strings.Split(vs.Destination, ":")
	ip := strings.Trim(s[0], partition)
	port, err := strconv.Atoi(s[1])
	if err != nil {
		return nil, fmt.Errorf("error reading F5 VS port: %v", err)
	}

	vip := &lbv1.VIP{
		Name: vs.Name,
		IP:   ip,
		Port: port,
		Pool: v.Pool,
	}

	return vip, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *Provider) CreateVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)

	config := &bigip.VirtualServer{
		Name:        v.Name,
		Partition:   partition,
		Destination: v.IP + ":" + strconv.Itoa(v.Port),
		Pool:        v.Pool,
		SourceAddressTranslation: struct {
			Type string "json:\"type,omitempty\""
			Pool string "json:\"pool,omitempty\""
		}{
			Type: "automap",
			Pool: "",
		},
		Profiles: []bigip.Profile{
			{
				Name:      "fastL4",
				FullPath:  "/Common/fastL4",
				Partition: "Common",
				Context:   "all",
			},
		},
	}
	err := p.f5.AddVirtualServer(config)
	if err != nil {
		return nil, fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, config, err)
	}
	return v, nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *Provider) EditVIP(v *lbv1.VIP) error {
	config := &bigip.VirtualServer{
		Name:        v.Name,
		Partition:   partition,
		Destination: v.IP + ":" + strconv.Itoa(v.Port),
		Pool:        v.Pool,
		SourceAddressTranslation: struct {
			Type string "json:\"type,omitempty\""
			Pool string "json:\"pool,omitempty\""
		}{
			Type: "automap",
			Pool: "",
		},
		Profiles: []bigip.Profile{
			{
				Name:      "fastL4",
				FullPath:  "/Common/fastL4",
				Partition: "Common",
				Context:   "all",
			},
		},
	}
	err := p.f5.PatchVirtualServer(partition+v.Name, config)
	if err != nil {
		return fmt.Errorf("error editing VIP %s: %v", v.Name, err)
	}
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *Provider) DeleteVIP(v *lbv1.VIP) error {
	err := p.f5.DeleteVirtualServer(v.Name)
	if err != nil {
		return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	}
	return nil
}
