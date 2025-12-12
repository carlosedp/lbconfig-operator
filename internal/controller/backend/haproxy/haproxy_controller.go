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
	"github.com/carlosedp/haproxy-go-client/client/bind"
	"github.com/carlosedp/haproxy-go-client/client/frontend"
	"github.com/carlosedp/haproxy-go-client/client/server"
	"github.com/carlosedp/haproxy-go-client/client/sites"
	"github.com/carlosedp/haproxy-go-client/client/transactions"
	"github.com/go-logr/logr"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/haproxytech/client-native/v4/models"
	"k8s.io/utils/ptr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	backend_controller "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_controller"
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
	monitor     lbv1.Monitor
	ctx         context.Context
	lbmethod    string
}

func init() {
	err := backend_controller.RegisterProvider("HAProxy", new(HAProxyProvider))
	if err != nil {
		panic(err)
	}
}

// We use round robin for the backend servers if least response is choosen since HAProxy doesn't have it.
// SOURCEIPHASH not enabled yet in CRD since it is not supported by F5.
var LBMethodMap = map[string]string{"ROUNDROBIN": "roundrobin", "LEASTCONNECTION": "leastconn", "LEASTRESPONSETIME": "roundrobin", "SOURCEIPHASH": "source"}

// Create creates a new Load Balancer backend provider
func (p *HAProxyProvider) Create(ctx context.Context, lbBackend lbv1.Provider, username string, password string) error {
	log := ctrllog.FromContext(ctx).WithValues("provider", "HAProxy")
	p.ctx = context.Background()
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
		_ = p.CloseError()
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
	p.log.Info("Commiting transaction", "transaction", p.transaction)
	_, _, err := p.haproxy.Transactions.CommitTransaction(&transactions.CommitTransactionParams{
		ID:          p.transaction,
		Context:     p.ctx,
		ForceReload: ptr.To[bool](true),
	}, p.auth)
	if err != nil {
		_ = p.CloseError()
		return err
	}
	return nil
}

// Close closes the connection to the Load Balancer
func (p *HAProxyProvider) CloseError() error {
	p.log.Info("Deleting transaction due error", "transaction", p.transaction)
	_, err := p.haproxy.Transactions.DeleteTransaction(&transactions.DeleteTransactionParams{
		ID:      p.transaction,
		Context: p.ctx,
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
	// Always return empty monior to force update
	return &lbv1.Monitor{}, nil
}

// CreateMonitor creates a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *HAProxyProvider) CreateMonitor(m *lbv1.Monitor) error {
	p.monitor = *m
	return nil
}

// EditMonitor edits a monitor in the IP Load Balancer
// if port argument is 0, no port override is configured
func (p *HAProxyProvider) EditMonitor(m *lbv1.Monitor) error {
	p.monitor = *m
	return nil

}

// DeleteMonitor deletes a monitor in the IP Load Balancer
func (p *HAProxyProvider) DeleteMonitor(m *lbv1.Monitor) error {
	p.monitor = lbv1.Monitor{}
	// Maybe call EditPool?
	return nil
}

// ----------------------------------------
// Pool Management
// ----------------------------------------

// GetPool gets a server pool from the Load Balancer
func (p *HAProxyProvider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	newPool, err := p.haproxy.Backend.GetBackend(&backend.GetBackendParams{
		Name:          pool.Name,
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil && !strings.Contains(err.Error(), "getBackendNotFound") {
		_ = p.CloseError()
		return nil, fmt.Errorf("error getting pool: %v", err)
	}

	// Return in case pool does not exist
	if newPool == nil {
		return nil, nil
	}

	retPool := &lbv1.Pool{
		Name:    newPool.Payload.Data.Name,
		Monitor: "changed",
	}

	return retPool, nil
}

// CreatePool creates a server pool in the Load Balancer
func (p *HAProxyProvider) CreatePool(pool *lbv1.Pool) error {
	_, _, err := p.haproxy.Backend.CreateBackend(&backend.CreateBackendParams{
		Data: &models.Backend{
			Name:     pool.Name,
			AdvCheck: "httpchk",
			Balance: &models.Balance{
				Algorithm: &p.lbmethod,
			},
			Mode: "tcp",
			HttpchkParams: &models.HttpchkParams{
				Method: "GET",
				URI:    p.monitor.Path,
			},
		},
		Context:       p.ctx,
		TransactionID: &p.transaction,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error creating pool %s: %v", pool.Name, err)
	}

	return nil
}

// EditPool modifies a server pool in the Load Balancer
func (p *HAProxyProvider) EditPool(pool *lbv1.Pool) error {
	// Create Pool with pre-existing monitor
	backendOK, _, err := p.haproxy.Backend.ReplaceBackend(&backend.ReplaceBackendParams{
		Name: pool.Name,
		Data: &models.Backend{
			Name:     pool.Name,
			AdvCheck: "httpchk",
			Balance: &models.Balance{
				Algorithm: &p.lbmethod,
			},
			Mode: "tcp",
			HttpchkParams: &models.HttpchkParams{
				Method: "GET",
				URI:    p.monitor.Path,
			},
		},
		Context:       p.ctx,
		TransactionID: &p.transaction,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error editing pool(ERR) %s: %v", pool.Name, err)
	}
	// Return in case pool does not exist
	if backendOK == nil {
		return nil
	}

	pool, err = p.GetPoolMembers(pool)
	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error editing pool(getting pool members) %s: %v", pool.Name, err)
	}
	if pool != nil {
		for _, m := range pool.Members {
			err = p.EditPoolMember(&m, pool, "enable")
			if err != nil {
				_ = p.CloseError()
				return fmt.Errorf("error editing pool(editing pool members) %s: %v", pool.Name, err)
			}
		}
	}
	return nil
}

// DeletePool removes a server pool in the Load Balancer
func (p *HAProxyProvider) DeletePool(pool *lbv1.Pool) error {
	_, _, err := p.haproxy.Backend.DeleteBackend(&backend.DeleteBackendParams{
		Name:          pool.Name,
		Context:       p.ctx,
		TransactionID: &p.transaction,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
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
		Backend:       &pool.Name,
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return nil, fmt.Errorf("error getting pool members: %v", err)
	}
	if poolMembers.Payload.Data == nil {
		return nil, nil
	}

	for _, member := range poolMembers.Payload.Data {
		ip := member.Address
		port := member.Port
		node := &lbv1.Node{
			Name: member.Name,
			Host: ip,
		}
		mem := &lbv1.PoolMember{
			Node: *node,
			Port: int(*port),
		}
		members = append(members, *mem)
	}
	pool.Members = members
	return pool, nil
}

// CreatePoolMember creates a member to be added to pool in the Load Balancer
func (p *HAProxyProvider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	server := &server.CreateServerParams{
		Backend: &pool.Name,
		Data: &models.Server{
			Name:    m.Node.Name,
			Address: m.Node.Host,
			Port:    ptr.To[int64](int64(m.Port)),
			ServerParams: models.ServerParams{
				Check:           "enabled",
				Inter:           ptr.To[int64](1000), // in ms
				HealthCheckPort: ptr.To[int64](int64(p.monitor.Port)),
			},
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}
	if p.monitor.MonitorType == "https" {
		server.Data.CheckSsl = "enabled"
		server.Data.Verify = "none"
	}
	_, _, err := p.haproxy.Server.CreateServer(server, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error creating pool member: %v", err)
	}
	p.log.Info("Created node", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// EditPoolMember modifies a server pool member in the Load Balancer
// status could be "enable" or "disable"
func (p *HAProxyProvider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	maintenanceStatus := func() string {
		if status == "enable" {
			return "disabled"
		} else {
			return "enabled"
		}
	}()

	server := &server.ReplaceServerParams{
		Backend: &pool.Name,
		Name:    m.Node.Name,
		Data: &models.Server{
			Name:    m.Node.Name,
			Address: m.Node.Host,
			Port:    ptr.To[int64](int64(m.Port)),
			ServerParams: models.ServerParams{
				Check:           "enabled",
				Maintenance:     maintenanceStatus,
				Inter:           ptr.To[int64](1000),
				HealthCheckPort: ptr.To[int64](int64(p.monitor.Port)),
			},
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}
	if p.monitor.MonitorType == "https" {
		server.Data.CheckSsl = "enabled"
		server.Data.Verify = "none"
	}
	_, _, err := p.haproxy.Server.ReplaceServer(server, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error editing pool member: %v", err)
	}
	p.log.Info("Edited node", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// DeletePoolMember deletes a member in the Load Balancer
func (p *HAProxyProvider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {

	_, _, err := p.haproxy.Server.DeleteServer(&server.DeleteServerParams{
		Backend:       &pool.Name,
		Name:          m.Node.Name,
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error deleting pool member: %v", err)
	}
	p.log.Info("Deleted node", "node", m.Node.Name, "host", m.Node.Host)
	return nil
}

// ----------------------------------------
// VIP Management
// ----------------------------------------

// GetVIP gets a VIP in the IP Load Balancer
func (p *HAProxyProvider) GetVIP(v *lbv1.VIP) (*lbv1.VIP, error) {
	getFrontend, err := p.haproxy.Frontend.GetFrontend(&frontend.GetFrontendParams{
		Name:          v.Name,
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil && !strings.Contains(err.Error(), "getFrontendNotFound") {
		_ = p.CloseError()
		return nil, fmt.Errorf("error getting haproxy frontend %s: %v", v.Name, err)
	}

	// // Return in case VIP does not exist
	if getFrontend == nil {
		return nil, nil
	}
	getFrontendBind, err := p.haproxy.Bind.GetBind(
		&bind.GetBindParams{
			Name:          v.Name,
			TransactionID: &p.transaction,
			ParentName:    &v.Name,
			ParentType:    ptr.To[string]("frontend"),
			Context:       p.ctx,
		}, p.auth)
	if err != nil {
		_ = p.CloseError()
		return nil, fmt.Errorf("error getting haproxy frontend bind %s: %v", v.Name, err)
	}

	vip := &lbv1.VIP{
		Name: getFrontend.Payload.Data.Name,
		IP:   getFrontendBind.Payload.Data.Address,
		Port: int(*getFrontendBind.Payload.Data.Port),
		Pool: getFrontend.Payload.Data.DefaultBackend,
	}
	return vip, nil
}

// CreateVIP creates a Virtual Server in the Load Balancer
func (p *HAProxyProvider) CreateVIP(v *lbv1.VIP) error {
	// Create frontend
	_, _, err := p.haproxy.Frontend.CreateFrontend(&frontend.CreateFrontendParams{
		Data: &models.Frontend{
			Name:           v.Name,
			Mode:           "tcp",
			DefaultBackend: v.Pool,
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error creating frontend: %v", err)
	}

	// Create frontend binds
	_, _, err = p.haproxy.Bind.CreateBind(&bind.CreateBindParams{
		Frontend: &v.Name,
		Data: &models.Bind{
			BindParams: models.BindParams{
				Name: v.Name,
			},
			Address: v.IP,
			Port:    ptr.To[int64](int64(v.Port)),
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error creating frontend bind: %v", err)
	}

	p.log.Info("Created VIP", "VIP", v.Name)
	return nil
}

// EditVIP modifies a Virtual Server in the Load Balancer
func (p *HAProxyProvider) EditVIP(v *lbv1.VIP) error {

	// Edit frontend
	_, _, err := p.haproxy.Frontend.ReplaceFrontend(&frontend.ReplaceFrontendParams{
		Data: &models.Frontend{
			Name: v.Name,
			Mode: "http",
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error editing frontend: %v", err)
	}

	// Edit frontend binds
	_, _, err = p.haproxy.Bind.ReplaceBind(&bind.ReplaceBindParams{
		Data: &models.Bind{
			BindParams: models.BindParams{
				Name: v.Name,
			},
			Address: v.IP,
			Port:    ptr.To[int64](int64(v.Port)),
		},
		TransactionID: &p.transaction,
		Context:       p.ctx,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error editing frontend bind: %v", err)
	}
	return nil
}

// DeleteVIP deletes a Virtual Server in the Load Balancer
func (p *HAProxyProvider) DeleteVIP(v *lbv1.VIP) error {
	_, _, err := p.haproxy.Frontend.DeleteFrontend(&frontend.DeleteFrontendParams{
		Name:          v.Name,
		Context:       p.ctx,
		TransactionID: &p.transaction,
	}, p.auth)

	if err != nil {
		_ = p.CloseError()
		return fmt.Errorf("error deleting VIP %s: %v", v.Name, err)
	}
	return nil
}
