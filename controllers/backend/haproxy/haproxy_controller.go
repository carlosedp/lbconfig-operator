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

package haproxy

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/carlosedp/haproxy-go-client/client"
	"github.com/carlosedp/haproxy-go-client/client/backend"
	"github.com/carlosedp/haproxy-go-client/client/frontend"
	"github.com/carlosedp/haproxy-go-client/client/server"
	"github.com/carlosedp/haproxy-go-client/client/sites"
	"github.com/carlosedp/haproxy-go-client/client/transactions"
	"github.com/go-logr/logr"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/haproxytech/client-native/v4/models"
	"k8s.io/utils/pointer"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	backend_controller "github.com/carlosedp/lbconfig-operator/controllers/backend/backend_controller"
)

// ----------------------------------------
// Provider creation and connection
// ----------------------------------------

// Provider is the object for the HAProxy Provider implementing the Provider interface
type HAProxyProvider struct {
	log         logr.Logger
	haproxy     *client.DataPlane
	host        string
	hostport    int
	username    string
	password    string
	auth        runtime.ClientAuthInfoWriter
	transaction string
	version     int64
	monitor     *models.HTTPCheck
	ctx         context.Context
	lbmethod    string
}

func init() {
	backend_controller.RegisterProvider("HAProxy", new(HAProxyProvider))
}

// We use round robin for the backend servers if least response is choosen since HAProxy doesn't have it.
var LBMethodMap = map[string]string{"ROUNDROBIN": "roundrobin", "LEASTCONNECTION": "leastconn", "LEASTRESPONSETIME": "roundrobin"}

// Create creates a new Load Balancer backend provider
func (p *HAProxyProvider) Create(ctx context.Context, lbBackend lbv1.Provider, username string, password string) error {
	log := ctrllog.FromContext(ctx)
	p.ctx = context.Background()
	log.WithValues("provider", "HAProxy")
	p.log = log
	p.host = lbBackend.Host
	p.hostport = lbBackend.Port
	p.username = username
	p.password = password
	p.auth = httptransport.BasicAuth(p.username, p.password)
	p.lbmethod = LBMethodMap[lbBackend.LBMethod]

	c, _ := url.Parse(p.host)
	host := c.Host + ":" + fmt.Sprintf("%d", p.hostport)

	t, _ := httptransport.TLSTransport(httptransport.TLSClientOptions{
		InsecureSkipVerify: !lbBackend.ValidateCerts,
	})

	transport := httptransport.New(host, "/v2", []string{c.Scheme})
	transport.Transport = t
	transport.DefaultAuthentication = p.auth
	transport.Debug = lbBackend.Debug

	// create the API client, with the transport
	p.haproxy = client.New(transport, strfmt.Default)
	return nil
}

// Connect creates a connection to the IP Load Balancer
func (p *HAProxyProvider) Connect() error {

	// Use Sites to grab the current config version of the HAProxy
	sites, err := p.haproxy.Sites.GetSites(&sites.GetSitesParams{Context: p.ctx}, p.auth)
	if err != nil {
		return err
	}
	p.version = sites.Payload.Version
	p.log.Info("Got HAProxy config version", "version", p.version)

	// Create a new transaction with previous version
	t, err := p.haproxy.Transactions.StartTransaction(&transactions.StartTransactionParams{
		Version: p.version,
		Context: p.ctx,
	}, p.auth)
	if err != nil {
		return err
	}
	p.transaction = t.Payload.ID

	return nil
}

// HealthCheck checks if a connection to the Load Balancer is established
func (p *HAProxyProvider) HealthCheck() error {
	return nil
}

// Close closes the connection to the Load Balancer
func (p *HAProxyProvider) Close() error {
	_, _, err := p.haproxy.Transactions.CommitTransaction(&transactions.CommitTransactionParams{
		ID: p.transaction,
	}, p.auth)
	if err != nil {
		return err
	}
	return nil
}

// ----------------------------------------
// Monitor Management
// ----------------------------------------

// GetMonitor gets a monitor in the IP Load Balancer
func (p *HAProxyProvider) GetMonitor(monitor *lbv1.Monitor) (*lbv1.Monitor, error) {
	// Return in case monitor is not set
	if p.monitor == nil {
		return nil, nil
	}

	// Return monitor details in case it exists
	mon := &lbv1.Monitor{
		Name:        "no-name-monitor", // Fix this
		MonitorType: p.monitor.Proto,
		Path:        p.monitor.URI,
		Port:        int(*p.monitor.Port),
	}

	return mon, nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *HAProxyProvider) CreateMonitor(m *lbv1.Monitor) error {

	p.monitor = &models.HTTPCheck{
		URI:    m.Path,
		Port:   pointer.Int64(int64(m.Port)),
		Index:  pointer.Int64(1),
		Method: "GET",
		Proto:  m.MonitorType,
		Type:   "send",
		// Add name from m.Name
	}
	return nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *HAProxyProvider) EditMonitor(m *lbv1.Monitor) error {
	p.monitor = &models.HTTPCheck{
		URI:    m.Path,
		Port:   pointer.Int64(int64(m.Port)),
		Index:  pointer.Int64(1),
		Method: "GET",
		Proto:  m.MonitorType,
		Type:   "send",
	}
	// Maybe call EditPool?
	return nil

}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *HAProxyProvider) DeleteMonitor(m *lbv1.Monitor) error {
	p.monitor = nil
	// Maybe call EditPool?
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *HAProxyProvider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	newPool, err := p.haproxy.Backend.GetBackend(&backend.GetBackendParams{
		Name:    pool.Name,
		Context: p.ctx,
	}, nil)

	if err != nil && !strings.Contains(err.Error(), "getBackendNotFound") {
		p.DeleteTransaction()
		return nil, fmt.Errorf("error getting pool: %v", err)
	}

	// Return in case pool does not exist
	if newPool == nil {
		p.log.Info("Pool does not exist")
		return nil, nil
	}

	retPool := &lbv1.Pool{
		// Name: newPool.Payload.Data.Name,
		// Monitor: newPool.Payload.Data.,
		// Members: members,
	}

	return retPool, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *HAProxyProvider) CreatePool(pool *lbv1.Pool) error {

	// m := &models.HTTPCheck{}
	// Create Pool with pre-existing monitor
	// if p.monitor != nil {
	// m = p.monitor
	// }

	_, _, err := p.haproxy.Backend.CreateBackend(&backend.CreateBackendParams{
		Data: &models.Backend{
			Name:      pool.Name,
			HTTPCheck: p.monitor,
			Balance: &models.Balance{
				Algorithm: &p.lbmethod,
			},
		},
		Context:       p.ctx,
		TransactionID: &p.transaction,
	}, nil)
	if err != nil {
		p.DeleteTransaction()
		return fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	return nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *HAProxyProvider) EditPool(pool *lbv1.Pool) error {
	m := &models.HTTPCheck{}
	// Create Pool with pre-existing monitor
	if p.monitor != nil {
		m = p.monitor
	}
	_, _, err := p.haproxy.Backend.ReplaceBackend(&backend.ReplaceBackendParams{
		Data: &models.Backend{
			Name:      pool.Name,
			HTTPCheck: m,
			Balance: &models.Balance{
				Algorithm: &p.lbmethod,
			},
		},
		Context: p.ctx,
	}, nil)
	if err != nil {
		p.DeleteTransaction()
		return fmt.Errorf("error editing pool(ERR) %s: %v", pool.Name, err)
	}

	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *HAProxyProvider) DeletePool(pool *lbv1.Pool) error {
	_, _, err := p.haproxy.Backend.DeleteBackend(&backend.DeleteBackendParams{
		Name:    pool.Name,
		Context: p.ctx,
	}, nil)

	if err != nil {
		p.DeleteTransaction()
		return fmt.Errorf("error deleting pool %s: %v", pool.Name, err)
	}
	return nil
}

// ----------------------------------------
// Pool Member Management
// ----------------------------------------

// GetPoolMembers gets the pool members and return them in Pool object
func (p *HAProxyProvider) GetPoolMembers(pool *lbv1.Pool) (*lbv1.Pool, error) {

	// // Get pool members
	var members []lbv1.PoolMember
	poolMembers, err := p.haproxy.Server.GetServers(&server.GetServersParams{
		Backend: &pool.Name,
		Context: p.ctx,
	}, nil)

	if err != nil {
		p.DeleteTransaction()
		return nil, fmt.Errorf("error getting pool members: %v", err)
	}

	for _, member := range poolMembers.Payload.Data {
		ip := member.Address
		port := int(*member.Port)
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
	pool.Members = members
	return pool, nil
}

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *HAProxyProvider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {

	// _, _, err := p.haproxy.Server.CreateServer(&server.CreateServerParams{
	// 	Backend: &pool.Name,
	// 	Data: &models.Server{
	// 		Name:            m.Node.Name,
	// 		Address:         m.Node.Host,
	// 		HealthCheckPort: pointer.Int64(int64(m.Port)),
	// 	},
	// 	TransactionID: &p.transaction,
	// 	Context:       p.ctx,
	// }, nil)
	// p.log.Info("Creating Node", "node", m.Node.Name, "host", m.Node.Host)

	// // Query node by IP
	// n, _ := p.f5.GetNode(m.Node.Host)
	// if n != nil {
	// 	p.f5.ModifyNode(m.Node.Host, &config)
	// } else {
	// 	err := p.f5.CreateNode(m.Node.Host, m.Node.Host)
	// 	if err != nil {
	// 		return fmt.Errorf("error creating node %s: %v", m.Node.Host, err)
	// 	}
	// }

	// err := p.f5.AddPoolMember(pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port))
	// if err != nil {
	// 	return fmt.Errorf("error adding member %s to pool %s: %v", m.Node.Host, pool.Name, err)
	// }

	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *HAProxyProvider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	// err := p.f5.PoolMemberStatus(pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port), status)
	// if err != nil {
	// 	return fmt.Errorf("error editing member %s in pool %s: %v", m.Node.Host, pool.Name, err)
	// }
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *HAProxyProvider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	// First delete member from pool
	// err := p.f5.DeletePoolMember(partition+pool.Name, m.Node.Host+":"+strconv.Itoa(m.Port))
	// if err != nil {
	// 	return fmt.Errorf("error removing member %s from pool %s: %v", m.Node.Host, pool.Name, err)
	// }
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
func (p *HAProxyProvider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	getFrontend, err := p.haproxy.Frontend.GetFrontend(&frontend.GetFrontendParams{
		Name:    v.Name,
		Context: p.ctx,
	}, nil)

	if err != nil {
		p.DeleteTransaction()
		return nil, fmt.Errorf("error getting haproxy frontend %s: %v", v.Name, err)
	}

	// // Return in case VIP does not exist
	if getFrontend == nil {
		return nil, nil
	}

	// Return VIP details in case it exists
	// s := getFrontend.Payload.Data.Name
	// ip := getFrontend.Payload.Data

	// s := strings.Split(vs.Destination, ":")
	// ip := strings.Trim(s[0], partition)
	// port, err := strconv.Atoi(s[1])
	// if err != nil {
	// 	return nil, fmt.Errorf("error reading F5 VS port: %v", err)
	// }

	// vip := &lbv1.VIP{
	// 	Name: s,
	// 	IP:   ip,
	// 	Port: port,
	// 	Pool: v.Pool,
	// }

	return nil, nil
	// return vip, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *HAProxyProvider) CreateVIP(v *lbv1.VIP) error {
	// The second parameter is our destination, and the third is the mask. You can use CIDR notation if you wish (as shown here)

	// config := &bigip.VirtualServer{
	// 	Name:        v.Name,
	// 	Partition:   partition,
	// 	Destination: v.IP + ":" + strconv.Itoa(v.Port),
	// 	Pool:        v.Pool,
	// 	SourceAddressTranslation: struct {
	// 		Type string "json:\"type,omitempty\""
	// 		Pool string "json:\"pool,omitempty\""
	// 	}{
	// 		Type: "automap",
	// 		Pool: "",
	// 	},
	// 	Profiles: []bigip.Profile{
	// 		{
	// 			Name:      "fastL4",
	// 			FullPath:  "/Common/fastL4",
	// 			Partition: "Common",
	// 			Context:   "all",
	// 		},
	// 	},
	// }
	// err := p.f5.AddVirtualServer(config)
	// if err != nil {
	// 	return nil, fmt.Errorf("error creating VIP %s, %+v: %v", v.Name, config, err)
	// }
	// return v, nil
	return nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *HAProxyProvider) EditVIP(v *lbv1.VIP) error {
	// config := &bigip.VirtualServer{
	// 	Name:        v.Name,
	// 	Partition:   partition,
	// 	Destination: v.IP + ":" + strconv.Itoa(v.Port),
	// 	Pool:        v.Pool,
	// 	SourceAddressTranslation: struct {
	// 		Type string "json:\"type,omitempty\""
	// 		Pool string "json:\"pool,omitempty\""
	// 	}{
	// 		Type: "automap",
	// 		Pool: "",
	// 	},
	// 	Profiles: []bigip.Profile{
	// 		{
	// 			Name:      "fastL4",
	// 			FullPath:  "/Common/fastL4",
	// 			Partition: "Common",
	// 			Context:   "all",
	// 		},
	// 	},
	// }
	// err := p.f5.PatchVirtualServer(partition+v.Name, config)
	// if err != nil {
	// 	return fmt.Errorf("error editing VIP %s: %v", v.Name, err)
	// }
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *HAProxyProvider) DeleteVIP(v *lbv1.VIP) error {
	// err := p.f5.DeleteVirtualServer(v.Name)
	// if err != nil {
	// 	return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	// }
	return nil
}

func (p *HAProxyProvider) DeleteTransaction() error {
	p.haproxy.Transactions.DeleteTransaction(&transactions.DeleteTransactionParams{
		ID: p.transaction,
	}, p.auth)
	return nil
}
