/*
MIT License

Copyright (c) 2022 Carlos Eduardo de Paula

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package dummy

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	backend "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// Provider is the object for the dummy provider implementing the Provider interface
type DummyProvider struct {
	log      logr.Logger
	host     string
	hostport int
	username string
	password string
}

func init() {
	backend.RegisterProvider("Dummy", new(DummyProvider))
}

// Create creates a new Load Balancer backend provider
func (p *DummyProvider) Create(ctx context.Context, lbBackend lbv1.Provider, username string, password string) error {
	log := ctrllog.FromContext(ctx)
	log.WithValues("provider", "Dummy")

	p.log = log
	p.host = lbBackend.Host
	p.hostport = lbBackend.Port
	p.username = username
	p.password = password

	err := p.Connect()
	if err != nil {
		return err
	}
	return nil
}

// Connect creates a connection to the IP Load Balancer
func (p *DummyProvider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.log.Info("Connect to dummy backend request", "host", host)
	return nil
}

// Close closes the connection to the IP Load Balancer
func (p *DummyProvider) Close() error {
	p.log.Info("Close connection to dummy backend")
	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *DummyProvider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
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
func (p *DummyProvider) CreateMonitor(m *lbv1.Monitor) error {
	p.log.Info("Request to create a monitor in the dummy backend", "monitor", m)
	return nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *DummyProvider) EditMonitor(m *lbv1.Monitor) error {
	p.log.Info("Request to edit a monitor in the dummy backend", "monitor", m)
	return nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *DummyProvider) DeleteMonitor(m *lbv1.Monitor) error {
	p.log.Info("Request to delete a monitor in the dummy backend", "monitor", m)
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *DummyProvider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	p.log.Info("Get dummy backend server pool")

	return nil, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *DummyProvider) CreatePool(pool *lbv1.Pool) error {
	p.log.Info("Creating Pool", "pool", pool.Name)
	return nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *DummyProvider) EditPool(pool *lbv1.Pool) error {
	p.log.Info("Editing Pool", "pool", pool.Name)
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *DummyProvider) DeletePool(pool *lbv1.Pool) error {
	p.log.Info("Deleting Pool", "pool", pool.Name)
	return nil
}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

func (p *DummyProvider) GetPoolMembers(pool *lbv1.Pool) (*lbv1.Pool, error) {
	p.log.Info("Get dummy backend server pool")

	return nil, nil
}

// GetPoolMembers gets the pool members and return them in Pool object
func (p *DummyProvider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *DummyProvider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	p.log.Info("Editing pool member", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *DummyProvider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Deleting pool member", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *DummyProvider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	p.log.Info("Get dummy backend VIP")
	return nil, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *DummyProvider) CreateVIP(v *lbv1.VIP) error {
	p.log.Info("Creating VIP", "vip", v.Name)
	return nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *DummyProvider) EditVIP(v *lbv1.VIP) error {
	p.log.Info("Editing VIP", "vip", v.Name)
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *DummyProvider) DeleteVIP(v *lbv1.VIP) error {
	p.log.Info("Deleting VIP", "vip", v.Name)
	return nil
}
