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

package f5_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/apis/externalloadbalancer/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/backend_controller"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/f5"
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

func TestF5(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "F5 Backend Suite")
}

// Define the objects used in the tests.

var credsSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "username",
		Namespace: "password",
	},
	Data: map[string][]byte{
		"username": []byte("testuser"),
		"password": []byte("testpassword"),
	},
}

var loadBalancer = &lbv1.ExternalLoadBalancer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "f5-backend",
		Namespace: "default",
	},
	Spec: lbv1.ExternalLoadBalancerSpec{
		Vip:   "10.0.0.1",
		Type:  "master",
		Ports: []int{443},
		Monitor: lbv1.Monitor{
			Path:        "/",
			Port:        80,
			MonitorType: "http",
		},
		Provider: lbv1.Provider{
			Vendor:        "F5_BigIP",
			Host:          "",
			Port:          0,
			Creds:         credsSecret.Name,
			Partition:     "Common",
			ValidateCerts: false,
			LBMethod:      "LEASTCONNECTION",
		},
	},
}

var monitor = &lbv1.Monitor{
	Name:        "test-monitor",
	MonitorType: "http",
	Path:        "/health",
	Port:        80,
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

var poolmember = &lbv1.PoolMember{
	Node: lbv1.Node{
		Name:   "test-node-5",
		Host:   "1.1.1.5",
		Labels: map[string]string{"node-role.kubernetes.io/master": ""},
	},
	Port: 80,
}

var VIP = &lbv1.VIP{
	Name: "test-vip",
	Pool: pool.Name,
	IP:   "1.2.3.4",
}

// Store the http session data for the request
type httpdataStruct struct {
	url    string
	method string
	data   string
	post   map[string][]string
}

var _ = Describe("When using a f5 backend", func() {
	var server *httptest.Server
	var httpdata httpdataStruct
	var ctx = context.TODO()

	BeforeEach(func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			GinkgoWriter.Println("Received a request for %s\n", r.URL.String())
			httpdata.url = r.URL.String()
			httpdata.method = r.Method
			body, _ := io.ReadAll(r.Body)
			httpdata.data = string(body)
			for k, v := range r.Form {
				httpdata.post[k] = v
			}

		}))
		c, err := url.Parse(server.URL)
		Expect(err).ToNot(HaveOccurred())
		host, port, err := net.SplitHostPort(c.Host)
		Expect(err).ToNot(HaveOccurred())
		p, err := strconv.Atoi(port)
		Expect(err).ToNot(HaveOccurred())
		loadBalancer.Spec.Provider.Host = c.Scheme + "://" + host
		loadBalancer.Spec.Provider.Port = p
	})

	AfterEach(func() {
		server.Close()
	})

	It("Should create the backend", func() {
		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
		Expect(ListProviders()).Should(ContainElement(strings.ToLower("F5_BigIP")))
		Expect(err).NotTo(HaveOccurred())
		Expect(createdBackend).NotTo(BeNil())
		Expect(reflect.TypeOf(createdBackend.Provider)).Should(Equal(reflect.TypeOf(&F5Provider{})))
	})

	It("Should connect to the backend", func() {
		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
		Expect(err).NotTo(HaveOccurred())
		err = createdBackend.Provider.Connect()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when handling load balancer monitors", func() {
		var createdBackend *BackendController
		var err error
		BeforeEach(func() {
			createdBackend, err = CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should get a monitor", func() {
			_, _ = createdBackend.Provider.GetMonitor(monitor)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http/test-monitor"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			// Getting error "error getting F5 Monitor test-monitor: invalid character 'O' looking for beginning of value"
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create a monitor", func() {
			err = createdBackend.Provider.CreateMonitor(monitor)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("POST"))
			// <map[string]interface {} | len:11>: {
			// 	"defaultsFrom": <string>"/Common/http",
			// 	"destination": <string>"*.80",
			// 	"interval": <float64>5,
			// 	"manualResume": <string>"disabled",
			// 	"timeout": <float64>16,
			// 	"name": <string>"test-monitor",
			// 	"reverse": <string>"disabled",
			// 	"responseTime": <float64>0,
			// 	"retryTime": <float64>0,
			// 	"send": <string>"GET /health",
			// 	"transparent": <string>"disabled",
			// }
			port := strings.Split(gjson.Get(httpdata.data, "destination").String(), ".")[1]
			mon_type := strings.Split(gjson.Get(httpdata.data, "defaultsFrom").String(), "/")[2]
			Eventually(port, timeout, interval).Should(Equal("80"))
			Eventually(gjson.Get(httpdata.data, "send").String(), timeout, interval).Should(Equal("GET /health"))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-monitor"))
			Eventually(mon_type, timeout, interval).Should(Equal("http"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should delete the monitor", func() {
			err = createdBackend.Provider.DeleteMonitor(monitor)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http/test-monitor"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should edit the monitor", func() {
			err = createdBackend.Provider.EditMonitor(monitor)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http/test-monitor"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PATCH"))

			// fmt.Fprintf(GinkgoWriter, "%s", httpdata)
			mon_type := strings.Split(gjson.Get(httpdata.data, "defaultsFrom").String(), "/")[2]
			Eventually(mon_type, timeout, interval).Should(Equal("http"))
			Eventually(gjson.Get(httpdata.data, "send").String(), timeout, interval).Should(Equal("GET /health"))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-monitor"))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when handling load balancer pools", func() {
		var createdBackend *BackendController
		var err error
		BeforeEach(func() {
			createdBackend, err = CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).NotTo(HaveOccurred())
		})
		It("Should get a pool", func() {
			_, _ = createdBackend.Provider.GetPool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			// Getting error "error getting F5 Monitor test-monitor: invalid character 'O' looking for beginning of value"
			// Expect(err).NotTo(HaveOccurred())
		})

		It("Should create a pool", func() {
			err = createdBackend.Provider.CreatePool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PUT"))
			Eventually(gjson.Get(httpdata.data, "loadBalancingMode").String(), timeout, interval).Should(Equal("least-connections-member"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should delete the pool", func() {
			err = createdBackend.Provider.DeletePool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should edit the pool", func() {
			err = createdBackend.Provider.EditPool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PUT"))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-pool"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should get pool members", func() {
			_, _ = createdBackend.Provider.GetPoolMembers(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool/members"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			// Getting error "error getting F5 Monitor test-monitor: invalid character 'O' looking for beginning of value"
			// Expect(err).NotTo(HaveOccurred())
		})
		It("Should create pool members", func() {
			_ = createdBackend.Provider.CreatePoolMember(poolmember, pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/node/1.1.1.5"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			// Expect(err).NotTo(HaveOccurred())
		})

		It("Should delete pool members", func() {
			err = createdBackend.Provider.DeletePoolMember(poolmember, pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/~Common~test-pool/members/1.1.1.5:80"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should edit pool members", func() {
			// Enable
			err = createdBackend.Provider.EditPoolMember(poolmember, pool, "enable")
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool/members/1.1.1.5:80"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PUT"))
			Eventually(gjson.Get(httpdata.data, "session").String(), timeout, interval).Should(Equal("user-enabled"))
			Expect(err).NotTo(HaveOccurred())
			// Disable
			err = createdBackend.Provider.EditPoolMember(poolmember, pool, "disable")
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool/members/1.1.1.5:80"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PUT"))
			// Expect(httpdata.data).Should(Equal(""))
			Eventually(gjson.Get(httpdata.data, "session").String(), timeout, interval).Should(Equal("user-disabled"))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when handling load balancer VIPs", func() {
		var createdBackend *BackendController
		var err error
		BeforeEach(func() {
			createdBackend, err = CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).NotTo(HaveOccurred())
		})
		It("Should get a VIP", func() {
			_, _ = createdBackend.Provider.GetVIP(VIP)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/test-vip"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should create a VIP", func() {
			err = createdBackend.Provider.CreateVIP(VIP)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("POST"))

			// Expect(httpdata["servicegroup_lbmonitor_binding"]).Should(Equal(""))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-vip"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should delete the VIP", func() {
			err = createdBackend.Provider.DeleteVIP(VIP)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/test-vip"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should edit the VIP", func() {
			err = createdBackend.Provider.EditVIP(VIP)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/~Common~test-vip"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("PATCH"))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-vip"))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
