package backend

import (
	"context"
	"fmt"
	"strconv"

	"github.com/scottdware/go-bigip"
)

// F5Backend is the F5 backend container
type F5Backend struct {
	f5       *bigip.BigIP
	provider F5Provider
}

//F5Provider is the container for the connection parameters
type F5Provider struct {
	host          string
	hostport      int
	username      string
	password      string
	partition     string
	validatecerts bool
}

// Connect creates a connection to the IP Load Balancer
func (b F5Backend) Connect() error {
	host := b.provider.host + ":" + strconv.Itoa(b.provider.hostport)
	b.f5 = bigip.NewSession(host, b.provider.username, b.provider.password, nil)

	return nil
}

// GetMonitor gets a monitor in the IP Load Balancer
func (b F5Backend) GetMonitor(ctx context.Context, name string) (string, error) {
	return "", nil
}

// GetPool gets a server pool in the IP Load Balancer
func (b F5Backend) GetPool(ctx context.Context, name string) (string, error) {
	_, err := b.f5.GetPool(name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// GetVIP gets a VIP in the IP Load Balancer
func (b F5Backend) GetVIP(ctx context.Context, name string) (string, error) {
	return "", nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (b F5Backend) CreateMonitor(ctx context.Context, name string, url string, port int) (string, error) {

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
	err := b.f5.AddMonitor(config, "http")
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	return name, nil
}

// EditMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (b F5Backend) EditMonitor(ctx context.Context, name string, url string, port int) (string, error) {
	return name, nil
}

// DeleteMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (b F5Backend) DeleteMonitor(ctx context.Context, name string, url string, port int) (string, error) {
	return name, nil
}

// CreateMember creates a member to be added to pool in the Load Balancer
func (b F5Backend) CreateMember(ctx context.Context, node string, IP string) (string, error) {
	err := b.f5.CreateNode(node, IP)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return node, nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (b F5Backend) EditPoolMember(ctx context.Context, name string, member string, port int, status string) (string, error) {
	err := b.f5.PoolMemberStatus(name, member+":"+strconv.Itoa(port), status)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (b F5Backend) DeletePoolMember(ctx context.Context, name string, member string, port int, status string) (string, error) {
	// First delete member from pool
	err := b.f5.DeletePoolMember(name, member+":"+strconv.Itoa(port))
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	// Then delete node (TEST)
	err = b.f5.DeleteNode(member)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// CreatePool creates a server pool in the Load Balancer
func (b F5Backend) CreatePool(ctx context.Context, name string, monitor string, members []string, port int) (string, error) {

	// Create Pool
	err := b.f5.CreatePool(name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}

	// Add members to Pool
	for _, m := range members {
		b.f5.AddPoolMember(name, m+":"+strconv.Itoa(port))
	}

	// Add monitor to Pool
	err = b.f5.AddMonitorToPool(monitor, name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// EditPool modifies a server pool in the Load Balancer
func (b F5Backend) EditPool(ctx context.Context, name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// DeletePool removes a server pool in the Load Balancer
func (b F5Backend) DeletePool(ctx context.Context, name string, monitor string, members []string, port int) (string, error) {
	return name, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (b F5Backend) CreateVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error) {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)
	err := b.f5.CreateVirtualServer(name, VIP, "0", pool, port)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return name, nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (b F5Backend) EditVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (b F5Backend) DeleteVIP(ctx context.Context, name string, VIP string, pool string, port int) (string, error) {
	return name, nil
}
