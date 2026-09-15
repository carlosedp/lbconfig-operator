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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// Graceful draining against real load balancers. These specs are skipped unless an environment variable
// points at a device API, with the credentials in the URL:
//
//	LBCONFIG_LIVE_HAPROXY=http://admin:admin@127.0.0.1:5555        (HAProxy Data Plane API v2)
//	LBCONFIG_LIVE_NETSCALER=http://nsroot:<password>@127.0.0.1:9080 (Citrix ADC / NetScaler CPX NITRO API)
//
// Unlike the simulated F5, they read the member state back from the device, so they catch requests the
// device answers but does not apply (or applies but rejects).

const (
	liveNodeLabel    = "lbconfig.carlosedp.com/live-test"
	liveDrainSeconds = 20 // long enough for the status to report the drain before it ends, NetScaler saves its config slowly
	liveTimeout      = 60 * time.Second
	liveInterval     = 500 * time.Millisecond
)

type liveNode struct {
	name string
	ip   string
}

// liveMember is the state of a pool member as reported by the load balancer
type liveMember struct {
	Exists   bool
	Disabled bool
}

type liveDevice struct {
	name     string
	vendor   string
	envVar   string
	vip      string
	port     int
	ipPrefix string
	// member reads the state of the node's member in the pool from the load balancer
	member func(api *url.URL, pool string, node liveNode, port int) (liveMember, error)
	// poolExists reports whether the pool is still configured in the load balancer
	poolExists func(api *url.URL, pool string) (bool, error)
}

// liveGet GETs a device API path and decodes a 200 response into v. Other statuses are returned without error.
func liveGet(api *url.URL, path string, header http.Header, v any) (int, error) {
	req, err := http.NewRequest(http.MethodGet, api.Scheme+"://"+api.Host+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header = header
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || v == nil {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(v)
}

func liveBasicAuth(api *url.URL) http.Header {
	password, _ := api.User.Password()
	req := &http.Request{Header: http.Header{}}
	req.SetBasicAuth(api.User.Username(), password)
	return req.Header
}

func liveNitroAuth(api *url.URL) http.Header {
	password, _ := api.User.Password()
	return http.Header{"X-Nitro-User": {api.User.Username()}, "X-Nitro-Pass": {password}}
}

func haproxyLiveDevice() liveDevice {
	return liveDevice{
		name:     "haproxy",
		vendor:   "HAProxy",
		envVar:   "LBCONFIG_LIVE_HAPROXY",
		vip:      "*",
		port:     8080,
		ipPrefix: "10.20.0.",
		// Servers are named after the node and disabled through maintenance mode
		member: func(api *url.URL, pool string, node liveNode, _ int) (liveMember, error) {
			var servers struct {
				Data []struct {
					Name        string `json:"name"`
					Maintenance string `json:"maintenance"`
				} `json:"data"`
			}
			status, err := liveGet(api, "/v2/services/haproxy/configuration/servers?backend="+url.QueryEscape(pool), liveBasicAuth(api), &servers)
			if err != nil || status == http.StatusNotFound {
				return liveMember{}, err
			}
			if status != http.StatusOK {
				return liveMember{}, fmt.Errorf("listing the servers of backend %s: HTTP %d", pool, status)
			}
			for _, s := range servers.Data {
				if s.Name == node.name {
					return liveMember{Exists: true, Disabled: s.Maintenance == "enabled"}, nil
				}
			}
			return liveMember{}, nil
		},
		poolExists: func(api *url.URL, pool string) (bool, error) {
			status, err := liveGet(api, "/v2/services/haproxy/configuration/backends/"+url.PathEscape(pool), liveBasicAuth(api), nil)
			if err != nil {
				return false, err
			}
			return status == http.StatusOK, nil
		},
	}
}

func netscalerLiveDevice() liveDevice {
	return liveDevice{
		name:     "netscaler",
		vendor:   "Citrix_ADC",
		envVar:   "LBCONFIG_LIVE_NETSCALER",
		vip:      "10.30.0.100",
		port:     80,
		ipPrefix: "10.30.0.",
		// Members are servicegroup bindings of a server named after the node IP
		member: func(api *url.URL, pool string, node liveNode, port int) (liveMember, error) {
			var bindings struct {
				Members []struct {
					Servername string `json:"servername"`
					Port       int    `json:"port"`
					State      string `json:"state"`
				} `json:"servicegroup_servicegroupmember_binding"`
			}
			status, err := liveGet(api, "/nitro/v1/config/servicegroup_servicegroupmember_binding/"+url.PathEscape(pool), liveNitroAuth(api), &bindings)
			if err != nil || status == http.StatusNotFound {
				return liveMember{}, err
			}
			if status != http.StatusOK {
				return liveMember{}, fmt.Errorf("listing the members of servicegroup %s: HTTP %d", pool, status)
			}
			for _, m := range bindings.Members {
				if m.Servername == node.ip && m.Port == port {
					return liveMember{Exists: true, Disabled: m.State == "DISABLED"}, nil
				}
			}
			return liveMember{}, nil
		},
		poolExists: func(api *url.URL, pool string) (bool, error) {
			status, err := liveGet(api, "/nitro/v1/config/servicegroup/"+url.PathEscape(pool), liveNitroAuth(api), nil)
			if err != nil {
				return false, err
			}
			return status == http.StatusOK, nil
		},
	}
}

var _ = Describe("Graceful connection draining against a live HAProxy", Ordered, func() {
	liveDrainSpecs(haproxyLiveDevice())
})

var _ = Describe("Graceful connection draining against a live Citrix ADC", Ordered, func() {
	liveDrainSpecs(netscalerLiveDevice())
})

func liveDrainSpecs(dev liveDevice) {
	ctx := context.Background()
	lbName := "live-" + dev.name
	credsName := lbName + "-creds"
	poolName := fmt.Sprintf("Pool-%s-%d", lbName, dev.port)
	lbKey := types.NamespacedName{Name: lbName, Namespace: Namespace}
	labels := map[string]string{liveNodeLabel: dev.name}
	node1 := liveNode{name: lbName + "-node-1", ip: dev.ipPrefix + "1"}
	node2 := liveNode{name: lbName + "-node-2", ip: dev.ipPrefix + "2"}
	enabled := liveMember{Exists: true}
	disabled := liveMember{Exists: true, Disabled: true}
	var api *url.URL

	member := func(n liveNode) func() (liveMember, error) {
		return func() (liveMember, error) { return dev.member(api, poolName, n, dev.port) }
	}

	drainingHosts := func(g Gomega) []string {
		lb := &lbv1.ExternalLoadBalancer{}
		g.Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		hosts := make([]string, 0, len(lb.Status.DrainingMembers))
		for _, dm := range lb.Status.DrainingMembers {
			hosts = append(hosts, dm.Node.Host)
		}
		return hosts
	}

	setLabel := func(n liveNode, present bool) {
		Eventually(func() error {
			node := &corev1.Node{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: n.name}, node); err != nil {
				return err
			}
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}
			if present {
				node.Labels[liveNodeLabel] = dev.name
			} else {
				delete(node.Labels, liveNodeLabel)
			}
			return k8sClient.Update(ctx, node)
		}, timeout, interval).Should(Succeed())
	}

	BeforeAll(func() {
		raw := os.Getenv(dev.envVar)
		if raw == "" {
			Skip("set " + dev.envVar + " to run against a live " + dev.vendor)
		}
		var err error
		api, err = url.Parse(raw)
		Expect(err).NotTo(HaveOccurred())
		port, err := strconv.Atoi(api.Port())
		Expect(err).NotTo(HaveOccurred())
		password, _ := api.User.Password()

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: credsName, Namespace: Namespace},
			Data:       map[string][]byte{"username": []byte(api.User.Username()), "password": []byte(password)},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(node1.name, labels, node1.ip))).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(node2.name, labels, node2.ip))).To(Succeed())
		Expect(k8sClient.Create(ctx, &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: Namespace},
			Spec: lbv1.ExternalLoadBalancerSpec{
				Vip:        dev.vip,
				NodeLabels: labels,
				Ports:      []int{dev.port},
				Monitor:    lbv1.Monitor{Path: "/healthz", Port: dev.port, MonitorType: "http"},
				Provider: lbv1.Provider{
					Vendor: dev.vendor,
					Host:   api.Scheme + "://" + api.Hostname(),
					Port:   port,
					Creds:  credsName,
				},
				Drain: &lbv1.DrainConfig{Enabled: true, TimeoutSeconds: liveDrainSeconds},
			},
		})).To(Succeed())
	})

	AfterAll(func() {
		if api == nil {
			return
		}
		lb := &lbv1.ExternalLoadBalancer{}
		if err := k8sClient.Get(ctx, lbKey, lb); err == nil {
			Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
			}, liveTimeout, liveInterval).Should(BeTrue())
		}
		for _, n := range []liveNode{node1, node2} {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: n.name}}))).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: credsName, Namespace: Namespace}}))).To(Succeed())
	})

	It("Should configure the pool with both members enabled", func() {
		Eventually(member(node1), liveTimeout, liveInterval).Should(Equal(enabled))
		Eventually(member(node2), liveTimeout, liveInterval).Should(Equal(enabled))
		Eventually(drainingHosts, liveTimeout, liveInterval).Should(BeEmpty())
	})

	It("Should disable a removed member and delete it after the drain timeout", func() {
		setLabel(node2, false)

		By("disabling the member while it drains")
		Eventually(member(node2), liveTimeout, liveInterval).Should(Equal(disabled))
		disabledAt := time.Now()
		Eventually(drainingHosts, liveTimeout, liveInterval).Should(ConsistOf(node2.ip))
		Consistently(member(node2), 2*time.Second, liveInterval).Should(Equal(disabled))
		Expect(member(node1)()).To(Equal(enabled))

		By("deleting the member once the drain timeout expired")
		Eventually(member(node2), liveTimeout, liveInterval).Should(Equal(liveMember{}))
		// Observed by polling, so allow for the polling interval on both ends
		Expect(time.Since(disabledAt)).To(BeNumerically(">=", liveDrainSeconds*time.Second-2*liveInterval))
		Eventually(drainingHosts, liveTimeout, liveInterval).Should(BeEmpty())
		Expect(member(node1)()).To(Equal(enabled))
	})

	It("Should re-enable a draining member when its node is added back", func() {
		By("using a drain timeout long enough to add the node back while it drains")
		Eventually(func() error {
			lb := &lbv1.ExternalLoadBalancer{}
			if err := k8sClient.Get(ctx, lbKey, lb); err != nil {
				return err
			}
			lb.Spec.Drain.TimeoutSeconds = 300
			return k8sClient.Update(ctx, lb)
		}, timeout, interval).Should(Succeed())

		setLabel(node1, false)
		Eventually(member(node1), liveTimeout, liveInterval).Should(Equal(disabled))
		Eventually(drainingHosts, liveTimeout, liveInterval).Should(ConsistOf(node1.ip))

		setLabel(node1, true)
		Eventually(member(node1), liveTimeout, liveInterval).Should(Equal(enabled))
		Eventually(drainingHosts, liveTimeout, liveInterval).Should(BeEmpty())
	})

	It("Should recreate a deleted member when its node is added back", func() {
		setLabel(node2, true)
		Eventually(member(node2), liveTimeout, liveInterval).Should(Equal(enabled))
	})

	It("Should remove the load balancer configuration when the instance is deleted", func() {
		lb := &lbv1.ExternalLoadBalancer{}
		Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
		}, liveTimeout, liveInterval).Should(BeTrue())
		// A reconciliation still reading the deleted instance from its cache must not configure it again
		Consistently(func() (bool, error) { return dev.poolExists(api, poolName) }, 5*time.Second, liveInterval).Should(BeFalse())
	})
}
