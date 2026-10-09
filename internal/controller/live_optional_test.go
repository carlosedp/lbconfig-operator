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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

// Optional VIP and monitor creation against real load balancers, skipped unless the same environment variables as
// the live draining specs point at a device API.

// liveExistingMonitor is a built-in NetScaler monitor the pools can use when the operator does not create its own
const liveExistingMonitor = "tcp"

// liveObjects reads the load balancer objects of an instance from a device
type liveObjects struct {
	dev liveDevice
	// vipExists reports whether the virtual server is configured
	vipExists func(api *url.URL, vip string) (bool, error)
	// monitorExists reports whether the monitor is configured, nil when the device has no monitor objects
	monitorExists func(api *url.URL, monitor string) (bool, error)
	// poolMonitors returns the monitors bound to the pool, nil when the device has no monitor objects
	poolMonitors func(api *url.URL, pool string) ([]string, error)
}

// liveExists GETs a device API path and reports whether the object exists
func liveExists(api *url.URL, path string, header http.Header) (bool, error) {
	status, err := liveGet(api, path, header, nil)
	if err != nil {
		return false, err
	}
	return status == http.StatusOK, nil
}

func haproxyLiveObjects() liveObjects {
	return liveObjects{
		dev: haproxyLiveDevice(),
		vipExists: func(api *url.URL, vip string) (bool, error) {
			return liveExists(api, "/v2/services/haproxy/configuration/frontends/"+url.PathEscape(vip), liveBasicAuth(api))
		},
	}
}

func netscalerLiveObjects() liveObjects {
	return liveObjects{
		dev: netscalerLiveDevice(),
		vipExists: func(api *url.URL, vip string) (bool, error) {
			return liveExists(api, "/nitro/v1/config/lbvserver/"+url.PathEscape(vip), liveNitroAuth(api))
		},
		monitorExists: func(api *url.URL, monitor string) (bool, error) {
			return liveExists(api, "/nitro/v1/config/lbmonitor/"+url.PathEscape(monitor), liveNitroAuth(api))
		},
		poolMonitors: func(api *url.URL, pool string) ([]string, error) {
			var bindings struct {
				Monitors []struct {
					Name string `json:"monitor_name"`
				} `json:"servicegroup_lbmonitor_binding"`
			}
			status, err := liveGet(api, "/nitro/v1/config/servicegroup_lbmonitor_binding/"+url.PathEscape(pool), liveNitroAuth(api), &bindings)
			if err != nil {
				return nil, err
			}
			if status != http.StatusOK {
				return nil, fmt.Errorf("listing the monitors of servicegroup %s: HTTP %d", pool, status)
			}
			monitors := []string{}
			for _, m := range bindings.Monitors {
				monitors = append(monitors, m.Name)
			}
			return monitors, nil
		},
	}
}

var _ = Describe("Optional VIP and monitor creation against a live HAProxy", Ordered, func() {
	liveOptionalSpecs(haproxyLiveObjects())
})

var _ = Describe("Optional VIP and monitor creation against a live Citrix ADC", Ordered, func() {
	liveOptionalSpecs(netscalerLiveObjects())
})

func liveOptionalSpecs(obj liveObjects) {
	ctx := context.Background()
	dev := obj.dev
	lbName := "live-optional-" + dev.name
	credsName := lbName + "-creds"
	port := dev.port + 1 // apart from the draining specs
	poolName := fmt.Sprintf("Pool-%s-%d", lbName, port)
	vipName := fmt.Sprintf("VIP-%s-%d", lbName, port)
	monitorName := "Monitor-" + lbName
	lbKey := types.NamespacedName{Name: lbName, Namespace: Namespace}
	labels := map[string]string{liveNodeLabel: lbName}
	node := liveNode{name: lbName + "-node-1", ip: dev.ipPrefix + "11"}
	enabled := liveMember{Exists: true}
	vip := dev.vip
	if vip != "*" {
		vip = dev.ipPrefix + "101"
	}
	var api *url.URL
	var provider lbv1.Provider

	spec := func() lbv1.ExternalLoadBalancerSpec {
		return lbv1.ExternalLoadBalancerSpec{
			Vip:        vip,
			NodeLabels: labels,
			Ports:      []int{port},
			Monitor:    lbv1.Monitor{Path: liveMonitorPath, Port: port, MonitorType: MonitorType},
			Provider:   provider,
		}
	}

	member := func() (liveMember, error) { return dev.member(api, poolName, node, port) }
	poolExists := func() (bool, error) { return dev.poolExists(api, poolName) }
	vipExists := func() (bool, error) { return obj.vipExists(api, vipName) }
	monitorExists := func(name string) func() (bool, error) {
		return func() (bool, error) { return obj.monitorExists(api, name) }
	}
	poolMonitors := func() ([]string, error) { return obj.poolMonitors(api, poolName) }

	updateSpec := func(change func(spec *lbv1.ExternalLoadBalancerSpec)) {
		Eventually(func() error {
			lb := &lbv1.ExternalLoadBalancer{}
			if err := k8sClient.Get(ctx, lbKey, lb); err != nil {
				return err
			}
			change(&lb.Spec)
			return k8sClient.Update(ctx, lb)
		}, timeout, interval).Should(Succeed())
	}

	getStatus := func(g Gomega) lbv1.ExternalLoadBalancerStatus {
		lb := &lbv1.ExternalLoadBalancer{}
		g.Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		return lb.Status
	}

	deleteInstance := func() {
		lb := &lbv1.ExternalLoadBalancer{}
		if err := k8sClient.Get(ctx, lbKey, lb); err != nil {
			return
		}
		Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
		}, liveTimeout, liveInterval).Should(BeTrue())
	}

	createInstance := func(change func(spec *lbv1.ExternalLoadBalancerSpec)) {
		s := spec()
		change(&s)
		Expect(k8sClient.Create(ctx, &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: Namespace},
			Spec:       s,
		})).To(Succeed())
	}

	BeforeAll(func() {
		raw := os.Getenv(dev.envVar)
		if raw == "" {
			Skip("set " + dev.envVar + " to run against a live " + dev.vendor)
		}
		var err error
		api, err = url.Parse(raw)
		Expect(err).NotTo(HaveOccurred())
		apiPort, err := strconv.Atoi(api.Port())
		Expect(err).NotTo(HaveOccurred())
		provider = lbv1.Provider{Vendor: dev.vendor, Host: api.Scheme + "://" + api.Hostname(), Port: apiPort, Creds: credsName}

		Expect(k8sClient.Create(ctx, liveCredsSecret(credsName, api))).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(node.name, labels, node.ip))).To(Succeed())
		createInstance(func(*lbv1.ExternalLoadBalancerSpec) {})
	})

	AfterAll(func() {
		if api == nil {
			return
		}
		deleteInstance()
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node.name}}))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: credsName, Namespace: Namespace}}))).To(Succeed())
	})

	It("Should create the VIP, the pool and the monitor by default", func() {
		Eventually(vipExists, liveTimeout, liveInterval).Should(BeTrue())
		Eventually(member, liveTimeout, liveInterval).Should(Equal(enabled))
		if obj.poolMonitors != nil {
			Eventually(poolMonitors, liveTimeout, liveInterval).Should(ConsistOf(monitorName))
		}
	})

	It("Should remove the VIP and keep the pool when VIP creation is turned off", func() {
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) { spec.CreateVIP = ptr.To(false) })
		Eventually(vipExists, liveTimeout, liveInterval).Should(BeFalse())
		Eventually(func(g Gomega) { g.Expect(getStatus(g).VIPs).To(BeEmpty()) }, liveTimeout, liveInterval).Should(Succeed())
		Expect(member()).To(Equal(enabled))
	})

	It("Should use an existing monitor and remove its own when monitor creation is turned off", func() {
		if obj.poolMonitors == nil {
			Skip(dev.vendor + " has no monitor objects")
		}
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) {
			spec.CreateMonitor = ptr.To(false)
			spec.Monitor.Name = liveExistingMonitor
		})
		Eventually(poolMonitors, liveTimeout, liveInterval).Should(ConsistOf(liveExistingMonitor))
		Eventually(monitorExists(monitorName), liveTimeout, liveInterval).Should(BeFalse())
		Eventually(func(g Gomega) { g.Expect(getStatus(g).Monitor).To(BeNil()) }, liveTimeout, liveInterval).Should(Succeed())
		Consistently(poolMonitors, 5*time.Second, liveInterval).Should(ConsistOf(liveExistingMonitor))
		Expect(member()).To(Equal(enabled))
		Expect(vipExists()).To(BeFalse())
	})

	It("Should remove the pool and keep the existing monitor when the instance is deleted", func() {
		deleteInstance()
		Eventually(poolExists, liveTimeout, liveInterval).Should(BeFalse())
		Consistently(poolExists, 5*time.Second, liveInterval).Should(BeFalse())
		if obj.monitorExists != nil {
			Expect(monitorExists(liveExistingMonitor)()).To(BeTrue())
		}
	})

	It("Should configure only the pool when created with VIP and monitor creation off", func() {
		if obj.poolMonitors == nil {
			Skip(dev.vendor + " has no monitor objects")
		}
		createInstance(func(spec *lbv1.ExternalLoadBalancerSpec) {
			spec.CreateVIP = ptr.To(false)
			spec.CreateMonitor = ptr.To(false)
			spec.Monitor.Name = liveExistingMonitor
		})
		Eventually(member, liveTimeout, liveInterval).Should(Equal(enabled))
		Eventually(poolMonitors, liveTimeout, liveInterval).Should(ConsistOf(liveExistingMonitor))
		Consistently(vipExists, 5*time.Second, liveInterval).Should(BeFalse())
		Expect(monitorExists(monitorName)()).To(BeFalse())
		deleteInstance()
		Eventually(poolExists, liveTimeout, liveInterval).Should(BeFalse())
	})

	It("Should not configure the load balancer when monitor creation is off on a provider without monitors", func() {
		if obj.poolMonitors != nil {
			Skip(dev.vendor + " supports existing monitors")
		}
		createInstance(func(spec *lbv1.ExternalLoadBalancerSpec) {
			spec.CreateMonitor = ptr.To(false)
			spec.Monitor.Name = liveExistingMonitor
		})
		Consistently(poolExists, 5*time.Second, liveInterval).Should(BeFalse())
		Expect(vipExists()).To(BeFalse())
		deleteInstance()
	})
}
