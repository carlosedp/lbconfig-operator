package backend

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

// F5Provider is the object for the F5 Big IP Provider implementing the Provider interface
type F5Provider struct {
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
func Create(lbBackend lbv1.LoadBalancerBackend, username string, password string) (*F5Provider, error) {
	var p = &F5Provider{
		host:          lbBackend.Spec.Provider.Host,
		hostport:      lbBackend.Spec.Provider.Port,
		partition:     lbBackend.Spec.Provider.Partition,
		validatecerts: *lbBackend.Spec.Provider.ValidateCerts,
		username:      username,
		password:      password,
	}
	p.log = p.log.WithValues("backend", lbBackend.Name, "provider", "F5")
	err := p.Connect()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Connect creates a connection to the IP Load Balancer
func (p *F5Provider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.f5 = bigip.NewSession(host, p.username, p.password, nil)

	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *F5Provider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
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
func (p *F5Provider) CreateMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {

	config := &bigip.Monitor{
		Name:          m.Name,
		ParentMonitor: "/Common/" + m.MonitorType,
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
func (p *F5Provider) EditMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {
	config := &bigip.Monitor{
		Name:          m.Name,
		ParentMonitor: "/Common/" + m.MonitorType,
		Interval:      5,
		Timeout:       16,
		SendString:    "GET " + m.Path,
		ReceiveString: "",
	}

	if m.Port != 0 {
		destination := "*." + strconv.Itoa(m.Port)
		config.Destination = destination
	}
	err := p.f5.PatchMonitor(m.Name, m.MonitorType, config)
	if err != nil {
		return nil, fmt.Errorf("error patching F5 monitor  %s: %v", m.Name, err)
	}
	return m, nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *F5Provider) DeleteMonitor(m *lbv1.Monitor) error {
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
func (p *F5Provider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {

	newPool, err := p.f5.GetPool(pool.Name)
	if err != nil {
		return nil, fmt.Errorf("error getting F5 pool: %v", err)
	}

	// Return in case pool does not exist
	if newPool == nil {
		return nil, nil
	}

	// Get pool members
	var members []lbv1.PoolMember
	poolMembers, _ := p.f5.PoolMembers(pool.Name)
	for _, member := range poolMembers.PoolMembers {
		fmt.Printf("Member: %+v", member)
		ip := strings.Split(member.Address, ":")[0]
		port, _ := strconv.Atoi(strings.Split(member.Address, ":")[1])
		mem := &lbv1.PoolMember{
			Name: member.Name,
			Host: ip,
			Port: port,
		}
		members = append(members, *mem)
	}

	retPool := &lbv1.Pool{
		Name:    newPool.Name,
		Monitor: newPool.Monitor,
		Members: members,
	}

	return retPool, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *F5Provider) CreatePool(pool *lbv1.Pool) (*lbv1.Pool, error) {

	// Create Pool
	err := p.f5.CreatePool(pool.Name)
	if err != nil {
		return nil, fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	// Add members to Pool
	for _, m := range pool.Members {
		err = p.f5.AddPoolMember(pool.Name, m.Host+":"+strconv.Itoa(m.Port))
		if err != nil {
			return nil, fmt.Errorf("error adding member %s to pool %s: %v", m.Host, pool.Name, err)
		}
	}

	// Add monitor to Pool
	err = p.f5.AddMonitorToPool(pool.Name, pool.Monitor)
	if err != nil {
		return nil, fmt.Errorf("error adding monitor %s to pool %s: %v", pool.Monitor, pool.Name, err)
	}
	return pool, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *F5Provider) EditPool(pool *lbv1.Pool) error {
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
func (p *F5Provider) DeletePool(pool *lbv1.Pool) error {
	return nil
}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *F5Provider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	err := p.f5.CreateNode(m.Name, m.Host)
	if err != nil {
		return fmt.Errorf("error creating node %s: %v", m.Host, err)
	}
	err = p.f5.AddPoolMember(pool.Name, m.Host+":"+strconv.Itoa(m.Port))
	if err != nil {
		return fmt.Errorf("error adding member %s to pool %s: %v", m.Host, pool.Name, err)
	}

	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *F5Provider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	err := p.f5.PoolMemberStatus(pool.Name, m.Name+":"+strconv.Itoa(m.Port), status)
	if err != nil {
		return fmt.Errorf("error editing member %s in pool %s: %v", m.Host, pool.Name, err)
	}
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *F5Provider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	// First delete member from pool
	err := p.f5.DeletePoolMember(pool.Name, m.Host+":"+strconv.Itoa(m.Port))
	if err != nil {
		return fmt.Errorf("error removing member %s from pool %s: %v", m.Host, pool.Name, err)
	}
	// Then delete node (TEST)
	err = p.f5.DeleteNode(m.Name)
	if err != nil {
		return fmt.Errorf("error deleting member %s: %v", m.Host, err)
	}
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *F5Provider) GetVIP(name string) (string, error) {
	return "", nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *F5Provider) CreateVIP(name string, VIP string, pool string, port int) (string, error) {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)
	err := p.f5.CreateVirtualServer(name, VIP, "0", pool, port)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *F5Provider) EditVIP(name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *F5Provider) DeleteVIP(name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}
