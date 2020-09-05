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

// Connect creates a connection to the F5 Big IP Load Balancer
func (b F5Backend) Connect() error {
	host := b.provider.host + ":" + strconv.Itoa(b.provider.hostport)
	b.f5 = bigip.NewSession(host, b.provider.username, b.provider.password, nil)

	return nil
}

// CreateMonitor creates a monitor in the F5 Big IP Load Balancer
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

// CreatePool creates a server pool in the F5 Big IP Load Balancer
func (b F5Backend) CreatePool(ctx context.Context, name string) (string, error) {
	_, err := b.f5.GetPool(name)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	b.f5.AddMonitorToPool("web_http", "web_80_pool")
	return name, nil
}

// CreateVIP creates a Virtual Server in the F5 Big IP Load Balancer
func (b F5Backend) CreateVIP(ctx context.Context, name string) (string, error) {
	return name, nil
}
