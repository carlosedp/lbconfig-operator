package backend

import (
	"fmt"
	"strconv"
	"strings"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/scottdware/go-bigip"
)

// F5Provider is the object for the F5 Big IP Provider
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

// GetMonitor gets a monitor in the IP Load Balancer
func (p *F5Provider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	m, err := p.f5.GetMonitor(monitor.Name, monitor.MonitorType)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("error getting F5 Monitor: %v", err)
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
		return nil, err
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
		return nil, err
	}
	return m, nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *F5Provider) DeleteMonitor(m *lbv1.Monitor) error {
	err := p.f5.DeleteMonitor(m.Name, m.MonitorType)
	if err != nil {
		return err
	}
	return nil
}

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
func (p *F5Provider) CreatePool(name string, monitor string, members []string, port int) (string, error) {

	// Create Pool
	err := p.f5.CreatePool(name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	// Add members to Pool
	for _, m := range members {
		p.f5.AddPoolMember(name, m+":"+strconv.Itoa(port))
	}

	// Add monitor to Pool
	err = p.f5.AddMonitorToPool(monitor, name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *F5Provider) EditPool(name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *F5Provider) DeletePool(name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// CreateMember creates a member to be added to pool in the Load Balancer
func (p *F5Provider) CreateMember(node string, IP string) (string, error) {
	err := p.f5.CreateNode(node, IP)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return node, nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *F5Provider) EditPoolMember(name string, member string, port int, status string) (string, error) {
	err := p.f5.PoolMemberStatus(name, member+":"+strconv.Itoa(port), status)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *F5Provider) DeletePoolMember(name string, member string, port int, status string) (string, error) {
	// First delete member from pool
	err := p.f5.DeletePoolMember(name, member+":"+strconv.Itoa(port))
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	// Then delete node (TEST)
	err = p.f5.DeleteNode(member)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

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
