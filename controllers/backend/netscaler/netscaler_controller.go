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

package netscaler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/citrix/adc-nitro-go/resource/config/basic"
	"github.com/citrix/adc-nitro-go/resource/config/lb"
	"github.com/citrix/adc-nitro-go/service"

	"github.com/go-logr/logr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	backend "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// NetscalerProvider is the object for the Citrix Netscaler NetscalerProvider implementing the NetscalerProvider interface
type NetscalerProvider struct {
	log           logr.Logger
	client        *service.NitroClient
	host          string
	hostport      int
	username      string
	password      string
	validatecerts bool
}

func init() {
	backend.RegisterProvider("netscaler", new(NetscalerProvider))
}

// Create creates a new Load Balancer backend provider
func (p *NetscalerProvider) Create(ctx context.Context, lbBackend lbv1.Provider, username string, password string) error {
	log := ctrllog.FromContext(ctx)
	log.WithValues("provider", "netscaler")

	if lbBackend.ValidateCerts == nil {
		return fmt.Errorf("validateCerts is required")
	}
	p.log = log
	p.host = lbBackend.Host
	p.hostport = lbBackend.Port
	p.validatecerts = *lbBackend.ValidateCerts
	p.username = username
	p.password = password

	var params = &service.NitroParams{
		Url:       "http://" + p.host + ":" + strconv.Itoa(p.hostport),
		Username:  p.username,
		Password:  p.password,
		SslVerify: p.validatecerts,
	}

	client, err := service.NewNitroClientFromParams(*params)

	if err != nil {
		return err
	}
	p.client = client
	return nil
}

// Connect creates a connection to the IP Load Balancer
func (p *NetscalerProvider) Connect() error {
	return nil
}

// Close closes the connection to the IP Load Balancer
func (p *NetscalerProvider) Close() error {
	return saveConfig(p, "close connection")
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *NetscalerProvider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	m, _ := p.client.FindResource(service.Lbmonitor.Type(), monitor.Name)

	// Return in case monitor does not exist
	if len(m) == 0 {
		return nil, nil
	}

	// Return monitor details in case it exists
	path := strings.TrimLeft(string(m["httprequest"].(string)), "GET ")

	mon := &lbv1.Monitor{
		Name:        m["monitorname"].(string),
		MonitorType: m["type"].(string),
		Path:        path,
		Port:        int(m["destport"].(float64)),
	}

	if m["secure"] == "YES" {
		mon.MonitorType = "https"
	}

	return mon, nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *NetscalerProvider) CreateMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {
	lbMonitor := lb.Lbmonitor{
		Monitorname: m.Name,
		Type:        "HTTP",
		Interval:    5,
		Downtime:    16,
		Httprequest: "GET " + m.Path,
	}

	if m.Port != 0 {
		lbMonitor.Destport = m.Port
	}
	// on Netscaler, HTTP and HTTPS are the same with different flags
	if m.MonitorType == "https" {
		lbMonitor.Secure = "YES"
		lbMonitor.Sslprofile = "ns_default_ssl_profile_backend"
	}

	name, err := p.client.AddResource(service.Lbmonitor.Type(), m.Name, &lbMonitor)
	if err != nil {
		return nil, fmt.Errorf("error creating Netscaler monitor %s: %v", name, err)
	}

	return m, nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *NetscalerProvider) EditMonitor(m *lbv1.Monitor) error {
	lbMonitor := lb.Lbmonitor{
		Monitorname: m.Name,
		Type:        "HTTP",
		Interval:    5,
		Downtime:    16,
		Httprequest: "GET " + m.Path,
	}

	if m.Port != 0 {
		lbMonitor.Destport = m.Port
	}
	// on Netscaler, HTTP and HTTPS are the same with different flags
	if m.MonitorType == "https" {
		lbMonitor.Secure = "YES"
		lbMonitor.Sslprofile = "ns_default_ssl_profile_backend"
	}

	name, err := p.client.AddResource(service.Lbmonitor.Type(), m.Name, &lbMonitor)
	if err != nil {
		return fmt.Errorf("error creating Netscaler monitor %s: %v", name, err)
	}
	return nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *NetscalerProvider) DeleteMonitor(m *lbv1.Monitor) error {
	// err := p.client.DeleteResource(service.Lbmonitor.Type(), m.Name)

	var t string
	if m.MonitorType == "https" {
		t = "HTTP"
	} else {
		t = m.MonitorType
	}
	var args = []string{
		"monitorname:" + m.Name,
		"type:" + t,
	}
	err := p.client.DeleteResourceWithArgs(service.Lbmonitor.Type(), m.Name, args)

	if err != nil {
		return fmt.Errorf("error deleting Netscaler monitor %s: %v", m.Name, err)
	}
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *NetscalerProvider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	m, _ := p.client.FindResource(service.Servicegroup.Type(), pool.Name)

	// Return in case pool does not exist
	if len(m) == 0 {
		p.log.Info("Pool does not exist")
		return nil, nil
	}
	// fmt.Printf("Pool: %+v\n", m)
	name := m["servicegroupname"].(string)
	monitor := ""

	// Get pool members
	var members []lbv1.PoolMember
	poolBinding, _ := p.client.FindResource(service.Servicegroup_binding.Type(), pool.Name)

	poolMembers := poolBinding["servicegroup_servicegroupmember_binding"]

	// Pool doesn't have members
	if poolMembers != nil {
		for _, member := range poolMembers.([]interface{}) {
			mem := member.(map[string]interface{})
			ip := mem["servername"].(string)
			port := int(mem["port"].(float64))
			name := ip + ":" + strconv.Itoa(port)
			// fmt.Printf(">>>>>> Member IP: %s: %+v\n", ip, mem)

			node := &lbv1.Node{
				Name: name,
				Host: ip,
			}
			pooMember := &lbv1.PoolMember{
				Node: *node,
				Port: port,
			}
			members = append(members, *pooMember)
		}
	}

	poolMonitor := poolBinding["servicegroup_lbmonitor_binding"].([]interface{})[0].(map[string]interface{})

	// Pool doesn't have a monitor
	if len(poolMonitor) != 0 {
		monitor = poolMonitor["monitor_name"].(string)
	}

	// fmt.Printf("Pool Monitor name %s: %+v\n", poolMonitor["monitor_name"].(string), poolMonitor)

	retPool := &lbv1.Pool{
		Name:    name,
		Monitor: monitor,
		Members: members,
	}

	return retPool, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *NetscalerProvider) CreatePool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	nsSvcGrp := &basic.Servicegroup{
		Servicegroupname: pool.Name,
		Servicetype:      "TCP",
	}
	_, err := p.client.AddResource(service.Servicegroup.Type(), pool.Name, nsSvcGrp)
	if err != nil {
		return nil, fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	monitorBinding := &basic.Servicegrouplbmonitorbinding{
		Servicegroupname: pool.Name,
		Monitorname:      pool.Monitor,
	}
	// Add monitor to Pool
	err = p.client.BindResource(service.Servicegroup.Type(), pool.Name, service.Lbmonitor.Type(), pool.Monitor, monitorBinding)
	if err != nil {
		return nil, fmt.Errorf("error adding monitor %s to pool %s: %v", pool.Monitor, pool.Name, err)
	}
	return pool, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *NetscalerProvider) EditPool(pool *lbv1.Pool) error {
	nsSvcGrp := &basic.Servicegroup{
		Servicegroupname: pool.Name,
		Servicetype:      "TCP",
	}
	_, err := p.client.AddResource(service.Servicegroup.Type(), pool.Name, nsSvcGrp)
	if err != nil {
		return fmt.Errorf("error editing pool %s: %v", pool.Name, err)
	}

	monitorBinding := &basic.Servicegrouplbmonitorbinding{
		Servicegroupname: pool.Name,
		Monitorname:      pool.Monitor,
	}
	// Add monitor to Pool
	err = p.client.BindResource(service.Servicegroup.Type(), pool.Name, service.Lbmonitor.Type(), pool.Monitor, monitorBinding)
	if err != nil {
		return fmt.Errorf("error editing pool %s, adding monitor %s: %v", pool.Name, pool.Monitor, err)
	}
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *NetscalerProvider) DeletePool(pool *lbv1.Pool) error {
	err := p.client.DeleteResource(service.Servicegroup.Type(), pool.Name)
	if err != nil {
		return fmt.Errorf("error deleting pool %s: %v", pool.Name, err)
	}
	return nil

}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *NetscalerProvider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)

	nsServer := basic.Server{
		Name:      m.Node.Host,
		Ipaddress: m.Node.Host,
	}
	_, err := p.client.AddResource(service.Server.Type(), m.Node.Host, &nsServer)
	if err != nil {
		return fmt.Errorf("error creating node %s: %v", m.Node.Host, err)
	}

	// Bind Service (member) to ServiceGroup (Pool)
	binding := basic.Servicegroupservicegroupmemberbinding{
		Servicegroupname: pool.Name,
		Servername:       m.Node.Host,
		Port:             m.Port,
	}
	_, err = p.client.AddResource(service.Servicegroup_servicegroupmember_binding.Type(), pool.Name, &binding)

	if err != nil {
		return fmt.Errorf("error adding member %s to pool %s: %v", m.Node.Host, pool.Name, err)
	}

	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *NetscalerProvider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *NetscalerProvider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	p.log.Info("Deleting Node", "node", m.Node.Name, "host", m.Node.Host)
	svcName := m.Node.Host

	// Unbind Service (member) from ServiceGroup (Pool)
	var args = []string{
		"servername:" + svcName,
		"servicegroupname:" + pool.Name,
		"port:" + strconv.Itoa(m.Port),
	}

	err := p.client.DeleteResourceWithArgs(service.Servicegroup_servicegroupmember_binding.Type(), pool.Name, args)

	if err != nil {
		return fmt.Errorf("error deleting member %s from pool %s: %v", m.Node.Host, pool.Name, err)
	}

	// Delete Server
	// Cannot delete server since it could also be used on another
	// LoadBalancer instance Pool. Get's removed from ServiceGroup once deleted

	// err = p.client.DeleteResource(service.Server.Type(), m.Node.Host)

	// if err != nil {
	// 	return fmt.Errorf("error deleting node %s: %v", m.Node.Host, err)
	// }

	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *NetscalerProvider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	vs, _ := p.client.FindResource(service.Lbvserver.Type(), v.Name)

	// Return in case VIP does not exist
	if len(vs) == 0 {
		return nil, nil
	}

	// fmt.Printf("VIP: %+v\n", vs)
	// Return VIP details in case it exists
	vip := &lbv1.VIP{
		Name: vs["name"].(string),
		IP:   vs["ipv46"].(string),
		Port: int(vs["port"].(float64)),
		Pool: v.Pool,
	}
	poolBinding, err := p.client.FindResource(service.Lbvserver_servicegroup_binding.Type(), v.Name)

	if err != nil && len(poolBinding) != 0 {
		poolName := poolBinding["servicegroupname"].(string)
		vip.Pool = poolName
	}

	return vip, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *NetscalerProvider) CreateVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	nsLB := lb.Lbvserver{
		Name:        v.Name,
		Ipv46:       v.IP,
		Port:        v.Port,
		Servicetype: "TCP",
		Lbmethod:    "ROUNDROBIN",
		// Lbmethod        : "LEASTCONNECTION",
	}
	_, err := p.client.AddResource(service.Lbvserver.Type(), v.Name, &nsLB)

	if err != nil {
		return nil, fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, nsLB, err)
	}

	binding := lb.Lbvserverservicegroupbinding{
		Servicegroupname: v.Pool,
		Name:             v.Name,
		//Weight				: weight,
	}
	err = p.client.BindResource(service.Lbvserver.Type(), v.Name, service.Servicegroup.Type(), v.Pool, &binding)
	if err != nil {
		return nil, fmt.Errorf("error binding ServiceGroup %s to VIP %s, %+v: %v", v.Pool, v.Name, nsLB, err)
	}

	return v, nil

}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *NetscalerProvider) EditVIP(v *lbv1.VIP) error {
	nsLB := lb.Lbvserver{
		Name:        v.Name,
		Ipv46:       v.IP,
		Port:        v.Port,
		Servicetype: "TCP",
		Lbmethod:    "ROUNDROBIN",
		// Lbmethod        : "LEASTCONNECTION",
	}
	_, err := p.client.AddResource(service.Lbvserver.Type(), v.Name, &nsLB)

	if err != nil {
		return fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, nsLB, err)
	}

	binding := lb.Lbvserverservicegroupbinding{
		Servicegroupname: v.Pool,
		Name:             v.Name,
		//Weight				: weight,
	}
	err = p.client.BindResource(service.Lbvserver.Type(), v.Name, service.Servicegroup.Type(), v.Pool, &binding)
	if err != nil {
		return fmt.Errorf("error binding ServiceGroup %s to VIP %s, %+v: %v", v.Pool, v.Name, nsLB, err)
	}

	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *NetscalerProvider) DeleteVIP(v *lbv1.VIP) error {
	err := p.client.DeleteResource(service.Lbvserver.Type(), v.Name)
	if err != nil {
		return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	}
	return nil
}

func saveConfig(p *NetscalerProvider, msg string) error {
	if err := p.client.SaveConfig(); err != nil {
		return fmt.Errorf("error saving Netscaler - %s: %v", msg, err)
	}
	p.log.Info("Configuration saved")
	return nil
}
