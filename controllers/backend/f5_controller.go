package backend

import (
	"fmt"
	"strconv"
	"strings"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/scottdware/go-bigip"
)

//F5Provider is the object for the F5 Big IP Provider
type F5Provider struct {
	f5            *bigip.BigIP
	host          string
	hostport      int
	username      string
	password      string
	partition     string
	validatecerts bool
}

// Connect creates a connection to the IP Load Balancer
func (p F5Provider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.f5 = bigip.NewSession(host, p.username, p.password, nil)

	return nil
}

// GetMonitor gets a monitor in the IP Load Balancer
func (p F5Provider) GetMonitor(name string, monitorType lbv1.Monitor) (lbv1.Monitor, error) {
	m, err := p.f5.GetMonitor(name, monitorType.MonitorType)
	if err != nil {
		fmt.Println(err)
		return lbv1.Monitor{}, fmt.Errorf("error getting F5 Monitor: %v", err)
	}
	s := strings.Split(m.Destination, ".")[1]
	port, err := strconv.Atoi(s)
	if err != nil {
		return lbv1.Monitor{}, fmt.Errorf("error converting F5 monitor port: %v", err)
	}
	mon := lbv1.Monitor{
		MonitorType: m.MonitorType,
		Path:        m.FullPath,
		Port:        port,
	}

	return mon, nil
}

// GetPool gets a server pool in the IP Load Balancer
func (p F5Provider) GetPool(name string) (string, error) {
	_, err := p.f5.GetPool(name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// GetVIP gets a VIP in the IP Load Balancer
func (p F5Provider) GetVIP(name string) (string, error) {
	return "", nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p F5Provider) CreateMonitor(name string, url string, port int) (string, error) {

	config := &bigip.Monitor{
		Name:          name,
		ParentMonitor: "http",
		Interval:      5,
		Timeout:       16,
		SendString:    "GET " + url + "\r\n",
		ReceiveString: "200 OK",
	}

	if port != 0 {
		destination := "*." + strconv.Itoa(port)
		config.Destination = destination
	}
	err := p.f5.AddMonitor(config, "http")
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	return name, nil
}

// EditMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p F5Provider) EditMonitor(name string, url string, port int) (string, error) {
	return name, nil
}

// DeleteMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p F5Provider) DeleteMonitor(name string, url string, port int) (string, error) {
	return name, nil
}

// CreateMember creates a member to be added to pool in the Load Balancer
func (p F5Provider) CreateMember(node string, IP string) (string, error) {
	err := p.f5.CreateNode(node, IP)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return node, nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p F5Provider) EditPoolMember(name string, member string, port int, status string) (string, error) {
	err := p.f5.PoolMemberStatus(name, member+":"+strconv.Itoa(port), status)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p F5Provider) DeletePoolMember(name string, member string, port int, status string) (string, error) {
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

// CreatePool creates a server pool in the Load Balancer
func (p F5Provider) CreatePool(name string, monitor string, members []string, port int) (string, error) {

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
func (p F5Provider) EditPool(name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// DeletePool removes a server pool in the Load Balancer
func (p F5Provider) DeletePool(name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p F5Provider) CreateVIP(name string, VIP string, pool string, port int) (string, error) {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)
	err := p.f5.CreateVirtualServer(name, VIP, "0", pool, port)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p F5Provider) EditVIP(name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p F5Provider) DeleteVIP(name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}
