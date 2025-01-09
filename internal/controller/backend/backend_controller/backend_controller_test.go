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

package controller_test

import (
	"context"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/lb.lbconfig.carlosedp.com/v1"
	. "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_controller"
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_loader"
	d "github.com/carlosedp/lbconfig-operator/internal/controller/backend/dummy"
)

func TestBackendController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backend Controller Suite")
}

var loadBalancer = &lbv1.ExternalLoadBalancer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "dummy-backend",
		Namespace: "default",
	},
	Spec: lbv1.ExternalLoadBalancerSpec{
		Vip: "10.0.0.1",
		Provider: lbv1.Provider{
			Vendor: "Dummy",
			Host:   "1.2.3.4",
			Port:   443,
			Creds:  "secretname",
		},
	},
}

var monitor = lbv1.Monitor{
	Path:        "/",
	Port:        80,
	MonitorType: "http",
}

var pool = &lbv1.Pool{
	Name: "test-pool",
	Members: []lbv1.PoolMember{{
		Node: lbv1.Node{
			Name:   "test-node-1",
			Host:   "1.1.1.1",
			Labels: map[string]string{"node-role.kubernetes.io/master": ""},
		},
		Port: 80},
		{
			Node: lbv1.Node{
				Name:   "test-node-2",
				Host:   "1.1.1.2",
				Labels: map[string]string{"node-role.kubernetes.io/master": ""},
			},
			Port: 80},
	},
}

var VIP = &lbv1.VIP{
	Name: "test-vip",
	Pool: pool.Name,
	IP:   "1.2.3.4",
}

var _ = Describe("Controllers/Backend/controller/backend_controller", func() {

	Context("When using a creating backends", func() {
		var ctx = context.TODO()

		It("Should validate that backend provider is registered", func() {
			Expect(ListProviders()).Should(ContainElement("dummy"))
		})

		It("Should return error if backend provider tries to register again", func() {
			err := RegisterProvider("Dummy", new(d.DummyProvider))
			Expect(err).To(MatchError(MatchRegexp("provider already exists.*")))
		})

		It("Should return error if backend provider does not exist", func() {
			loadBalancer := &lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-backend",
					Namespace: "default",
				},
				Spec: lbv1.ExternalLoadBalancerSpec{
					Vip: "10.0.0.1",
					Provider: lbv1.Provider{
						Vendor: "unknown",
						Host:   "1.2.3.4",
						Port:   443,
						Creds:  "secretname",
					},
				},
			}
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(MatchRegexp("no such provider.*")))
			Expect(createdBackend).To(BeNil())
		})

		It("Should create a provider with registered backend provider", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(BeNil())
			Expect(reflect.TypeOf(createdBackend.Provider)).Should(Equal(reflect.TypeOf(&d.DummyProvider{})))
		})

		It("Should handle a provider monitor", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(BeNil())
			err = createdBackend.HandleMonitors(ctx, &monitor)
			Expect(err).Should(BeNil())
		})

		It("Should handle a provider pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(BeNil())
			err = createdBackend.HandlePool(ctx, pool, &monitor)
			Expect(err).Should(BeNil())
		})

		It("Should handle a provider VIP", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(BeNil())
			err = createdBackend.HandleVIP(ctx, VIP)
			Expect(err).Should(BeNil())
		})

		It("Should handle a provider cleanup", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(BeNil())
			err = createdBackend.HandleCleanup(ctx, loadBalancer)
			Expect(err).Should(BeNil())
		})

	})
	Context("When using auxiliary functions", func() {
		It("Should return true if array contains member", func() {
			m := lbv1.PoolMember{
				Node: lbv1.Node{
					Name: "node1",
					Host: "1.1.1.1",
				},
				Port: 80,
			}
			a := []lbv1.PoolMember{m}
			output := ContainsMember(a, m)
			Expect(output).To(BeTrue())
		})

		It("Should return false if array doesn't contain member", func() {
			m := lbv1.PoolMember{
				Node: lbv1.Node{
					Name: "node1",
					Host: "1.1.1.1",
				},
				Port: 80,
			}
			m2 := m.DeepCopy()
			m2.Node.Host = "1.1.1.2"
			a := []lbv1.PoolMember{m}
			output := ContainsMember(a, *m2)
			Expect(output).To(BeFalse())
		})
	})
})
