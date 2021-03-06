package netscaler

import (
	"fmt"
	"strconv"
	"strings"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	"github.com/chiradeep/go-nitro/config/basic"
	"github.com/chiradeep/go-nitro/config/lb"
	"github.com/chiradeep/go-nitro/netscaler"
	"github.com/go-logr/logr"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// Provider is the object for the Citrix Netscaler Provider implementing the Provider interface
type Provider struct {
	log           logr.Logger
	client        *netscaler.NitroClient
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

	var params = &netscaler.NitroParams{
		Url:       "http://" + p.host,
		Username:  p.username,
		Password:  p.password,
		SslVerify: p.validatecerts,
	}

	client, err := netscaler.NewNitroClientFromParams(*params)

	if err != nil {
		return nil, err
	}
	p.client = client
	return p, nil
}

// Connect creates a connection to the IP Load Balancer
func (p *Provider) Connect() error {
	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *Provider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	m, _ := p.client.FindResource(netscaler.Lbmonitor.Type(), monitor.Name)

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
func (p *Provider) CreateMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) {
	defer saveConfig(p, "create monitor")
	lbMonitor := lb.Lbmonitor{
		Monitorname: m.Name,
		Type:        "HTTP",
		Interval:    5,
		Downtime:    16,
		Httprequest: "GET " + m.Path,
		Respcode:    "",
	}

	if m.Port != 0 {
		lbMonitor.Destport = m.Port
	}
	// on Netscaler, HTTP and HTTPS are the same with different flags
	if m.MonitorType == "https" {
		lbMonitor.Secure = "YES"
		lbMonitor.Sslprofile = "ns_default_ssl_profile_backend"
	}

	name, err := p.client.AddResource(netscaler.Lbmonitor.Type(), m.Name, &lbMonitor)
	if err != nil {
		return nil, fmt.Errorf("error creating Netscaler monitor %s: %v", name, err)
	}

	return m, nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *Provider) EditMonitor(m *lbv1.Monitor) error {
	defer saveConfig(p, "edit monitor")
	lbMonitor := lb.Lbmonitor{
		Monitorname: m.Name,
		Type:        "HTTP",
		Interval:    5,
		Downtime:    16,
		Httprequest: "GET " + m.Path,
		Respcode:    "",
	}

	if m.Port != 0 {
		lbMonitor.Destport = m.Port
	}
	// on Netscaler, HTTP and HTTPS are the same with different flags
	if m.MonitorType == "https" {
		lbMonitor.Secure = "YES"
		lbMonitor.Sslprofile = "ns_default_ssl_profile_backend"
	}

	name, err := p.client.AddResource(netscaler.Lbmonitor.Type(), m.Name, &lbMonitor)
	if err != nil {
		return fmt.Errorf("error creating Netscaler monitor %s: %v", name, err)
	}
	return nil
}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *Provider) DeleteMonitor(m *lbv1.Monitor) error {
	defer saveConfig(p, "delete monitor")
	// err := p.client.DeleteResource(netscaler.Lbmonitor.Type(), m.Name)

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
	err := p.client.DeleteResourceWithArgs(netscaler.Lbmonitor.Type(), m.Name, args)

	if err != nil {
		return fmt.Errorf("error deleting Netscaler monitor %s: %v", m.Name, err)
	}
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *Provider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	m, _ := p.client.FindResource(netscaler.Servicegroup.Type(), pool.Name)

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
	poolBinding, _ := p.client.FindResource(netscaler.Servicegroup_binding.Type(), pool.Name)

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
func (p *Provider) CreatePool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	defer saveConfig(p, "create pool")
	nsSvcGrp := &basic.Servicegroup{
		Servicegroupname: pool.Name,
		Servicetype:      "TCP",
	}
	_, err := p.client.AddResource(netscaler.Servicegroup.Type(), pool.Name, nsSvcGrp)
	if err != nil {
		return nil, fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	monitorBinding := &basic.Servicegrouplbmonitorbinding{
		Servicegroupname: pool.Name,
		Monitorname:      pool.Monitor,
	}
	// Add monitor to Pool
	err = p.client.BindResource(netscaler.Servicegroup.Type(), pool.Name, netscaler.Lbmonitor.Type(), pool.Monitor, monitorBinding)
	if err != nil {
		return nil, fmt.Errorf("error adding monitor %s to pool %s: %v", pool.Monitor, pool.Name, err)
	}
	return pool, nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *Provider) EditPool(pool *lbv1.Pool) error {
	defer saveConfig(p, "edit pool")
	nsSvcGrp := &basic.Servicegroup{
		Servicegroupname: pool.Name,
		Servicetype:      "TCP",
	}
	_, err := p.client.AddResource(netscaler.Servicegroup.Type(), pool.Name, nsSvcGrp)
	if err != nil {
		return fmt.Errorf("error editing pool %s: %v", pool.Name, err)
	}

	monitorBinding := &basic.Servicegrouplbmonitorbinding{
		Servicegroupname: pool.Name,
		Monitorname:      pool.Monitor,
	}
	// Add monitor to Pool
	err = p.client.BindResource(netscaler.Servicegroup.Type(), pool.Name, netscaler.Lbmonitor.Type(), pool.Monitor, monitorBinding)
	if err != nil {
		return fmt.Errorf("error editing pool %s, adding monitor %s: %v", pool.Name, pool.Monitor, err)
	}
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *Provider) DeletePool(pool *lbv1.Pool) error {
	defer saveConfig(p, "delete pool")
	err := p.client.DeleteResource(netscaler.Servicegroup.Type(), pool.Name)
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
	defer saveConfig(p, "create member")
	p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)

	nsServer := basic.Server{
		Name:      m.Node.Host,
		Ipaddress: m.Node.Host,
	}
	_, err := p.client.AddResource(netscaler.Server.Type(), m.Node.Host, &nsServer)
	if err != nil {
		return fmt.Errorf("error creating node %s: %v", m.Node.Host, err)
	}

	// Bind Service (member) to ServiceGroup (Pool)
	binding := basic.Servicegroupservicegroupmemberbinding{
		Servicegroupname: pool.Name,
		Servername:       m.Node.Host,
		Port:             m.Port,
	}
	_, err = p.client.AddResource(netscaler.Servicegroup_servicegroupmember_binding.Type(), pool.Name, &binding)

	if err != nil {
		return fmt.Errorf("error adding member %s to pool %s: %v", m.Node.Host, pool.Name, err)
	}

	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *Provider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *Provider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	defer saveConfig(p, "delete member")
	p.log.Info("Deleting Node", "node", m.Node.Name, "host", m.Node.Host)
	svcName := m.Node.Host

	// Unbind Service (member) from ServiceGroup (Pool)
	var args = []string{
		"servername:" + svcName,
		"servicegroupname:" + pool.Name,
		"port:" + strconv.Itoa(m.Port),
	}

	err := p.client.DeleteResourceWithArgs(netscaler.Servicegroup_servicegroupmember_binding.Type(), pool.Name, args)

	if err != nil {
		return fmt.Errorf("error deleting member %s from pool %s: %v", m.Node.Host, pool.Name, err)
	}

	// Delete Server
	// Cannot delete server since it could also be used on another
	// LoadBalancer instance Pool. Get's removed from ServiceGroup once deleted
	// err = p.client.DeleteResource(netscaler.Server.Type(), svcName)

	// if err != nil {
	// 	return fmt.Errorf("error deleting node %s: %v", m.Node.Host, err)
	// }
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *Provider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	vs, _ := p.client.FindResource(netscaler.Lbvserver.Type(), v.Name)

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
	poolBinding, err := p.client.FindResource(netscaler.Lbvserver_servicegroup_binding.Type(), v.Name)

	if err != nil && len(poolBinding) != 0 {
		poolName := poolBinding["servicegroupname"].(string)
		vip.Pool = poolName
	}

	return vip, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *Provider) CreateVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	defer saveConfig(p, "create VIP")
	nsLB := lb.Lbvserver{
		Name:        v.Name,
		Ipv46:       v.IP,
		Port:        v.Port,
		Servicetype: "TCP",
		Lbmethod:    "ROUNDROBIN",
		// Lbmethod        : "LEASTCONNECTION",
	}
	_, err := p.client.AddResource(netscaler.Lbvserver.Type(), v.Name, &nsLB)

	if err != nil {
		return nil, fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, nsLB, err)
	}

	binding := lb.Lbvserverservicegroupbinding{
		Servicegroupname: v.Pool,
		Name:             v.Name,
		//Weight				: weight,
	}
	err = p.client.BindResource(netscaler.Lbvserver.Type(), v.Name, netscaler.Servicegroup.Type(), v.Pool, &binding)
	if err != nil {
		return nil, fmt.Errorf("error binding ServiceGroup %s to VIP %s, %+v: %v", v.Pool, v.Name, nsLB, err)
	}

	return v, nil

}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *Provider) EditVIP(v *lbv1.VIP) error {
	defer saveConfig(p, "edit VIP")
	nsLB := lb.Lbvserver{
		Name:        v.Name,
		Ipv46:       v.IP,
		Port:        v.Port,
		Servicetype: "TCP",
		Lbmethod:    "ROUNDROBIN",
		// Lbmethod        : "LEASTCONNECTION",
	}
	_, err := p.client.AddResource(netscaler.Lbvserver.Type(), v.Name, &nsLB)

	if err != nil {
		return fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, nsLB, err)
	}

	binding := lb.Lbvserverservicegroupbinding{
		Servicegroupname: v.Pool,
		Name:             v.Name,
		//Weight				: weight,
	}
	err = p.client.BindResource(netscaler.Lbvserver.Type(), v.Name, netscaler.Servicegroup.Type(), v.Pool, &binding)
	if err != nil {
		return fmt.Errorf("error binding ServiceGroup %s to VIP %s, %+v: %v", v.Pool, v.Name, nsLB, err)
	}

	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *Provider) DeleteVIP(v *lbv1.VIP) error {
	defer saveConfig(p, "delete VIP")
	err := p.client.DeleteResource(netscaler.Lbvserver.Type(), v.Name)
	if err != nil {
		return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	}
	return nil
}

func saveConfig(p *Provider, msg string) error {
	if err := p.client.SaveConfig(); err != nil {
		return fmt.Errorf("error saving Netscaler - %s: %v", msg, err)
	}
	return nil
}
