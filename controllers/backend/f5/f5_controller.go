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

package f5

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/scottdware/go-bigip"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	backend "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// F5Provider is the object for the F5 Big IP F5Provider implementing the Provider interface
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

func init() {
	backend.RegisterProvider("F5_BigIP", new(F5Provider))
}

// Create creates a new Load Balancer backend provider
func (p *F5Provider) Create(ctx context.Context, lbBackend lbv1.Provider, username string, password string) error {
	log := ctrllog.FromContext(ctx)
	log.WithValues("provider", "F5_BigIP")

	if lbBackend.Partition == "" || lbBackend.ValidateCerts == nil {
		return fmt.Errorf("partition or validateCerts is required")
	}

	p.log = log
	p.host = lbBackend.Host
	p.hostport = lbBackend.Port
	p.partition = "/" + lbBackend.Partition + "/"
	p.validatecerts = *lbBackend.ValidateCerts
	p.username = username
	p.password = password

	return nil
}

// Connect creates a connection to the IP Load Balancer
func (p *F5Provider) Connect() error {
	host := p.host + ":" + strconv.Itoa(p.hostport)
	p.f5 = bigip.NewSession(host, p.username, p.password, nil)

	if err := p.HealthCheck(); err != nil {
		return fmt.Errorf("could not connect to f5 host '%s': %v", host, err)
	}
	return nil
}

// HealthCheck checks if a connection to the Load Balancer is established
func (p *F5Provider) HealthCheck() error {
	_, err := p.f5.Pools()
	if err != nil {
		return fmt.Errorf("failed to list f5 pools: %v", err)
	}
	return nil
}

// Close closes the connection to the IP Load Balancer
func (p *F5Provider) Close() error {
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
		ParentMonitor: p.partition + m.MonitorType,
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
func (p *F5Provider) EditMonitor(m *lbv1.Monitor) error {
	config := &bigip.Monitor{
		Name:          m.Name,
		ParentMonitor: p.partition + m.MonitorType,
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
		Monitor: strings.Trim(newPool.Monitor, p.partition),
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

	// Add monitor to Pool
	err = p.f5.AddMonitorToPool(pool.Monitor, pool.Name)
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
func (p *F5Provider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
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
func (p *F5Provider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	err := p.f5.PoolMemberStatus(pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port), status)
	if err != nil {
		return fmt.Errorf("error editing member %s in pool %s: %v", m.Node.Host, pool.Name, err)
	}
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *F5Provider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	// First delete member from pool
	err := p.f5.DeletePoolMember(p.partition+pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port))
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
func (p *F5Provider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
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
	ip := strings.Trim(s[0], p.partition)
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
func (p *F5Provider) CreateVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)

	config := &bigip.VirtualServer{
		Name:        v.Name,
		Partition:   p.partition,
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
func (p *F5Provider) EditVIP(v *lbv1.VIP) error {
	config := &bigip.VirtualServer{
		Name:        v.Name,
		Partition:   p.partition,
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
	err := p.f5.PatchVirtualServer(p.partition+v.Name, config)
	if err != nil {
		return fmt.Errorf("error editing VIP %s: %v", v.Name, err)
	}
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *F5Provider) DeleteVIP(v *lbv1.VIP) error {
	err := p.f5.DeleteVirtualServer(v.Name)
	if err != nil {
		return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	}
	return nil
}
