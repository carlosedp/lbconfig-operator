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

package haproxy_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	"k8s.io/utils/pointer"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/haproxy"
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

func TestHAProxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HAProxy Backend Suite")
}

// Create the backend Secret
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

// Create the ExternalLoadBalancer CRD
var loadBalancer = &lbv1.ExternalLoadBalancer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "haproxy-backend",
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
			Vendor:        "HAProxy",
			Host:          "",
			Port:          0,
			Creds:         credsSecret.Name,
			ValidateCerts: pointer.BoolPtr(false),
		},
	},
}

// Define the objects used in the tests.
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

var _ = Describe("When using a HAProxy backend", Ordered, func() {

	var server *httptest.Server
	var httpdata httpdataStruct
	var ctx = context.TODO()

	BeforeEach(func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			GinkgoWriter.Println("Received a request for %s\n", r.URL.String())
			httpdata.url = r.URL.String()
			httpdata.method = r.Method
			body, _ := ioutil.ReadAll(r.Body)
			httpdata.data = string(body)
			for k, v := range r.Form {
				httpdata.post[k] = v
			}

		}))
		connection := strings.Split(server.URL, ":")
		port, _ := strconv.Atoi(connection[len(connection)-1])
		loadBalancer.Spec.Provider.Host = connection[0] + ":" + connection[1]
		loadBalancer.Spec.Provider.Port = port
	})

	AfterEach(func() {
		server.Close()
	})

	It("Should create the backend", func() {
		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
		Expect(err).To(BeNil())
		Expect(createdBackend).NotTo(BeNil())
		Expect(ListProviders()).To(ContainElement(strings.ToLower("HAProxy")))
		Expect(reflect.TypeOf(createdBackend.Provider)).To(Equal(reflect.TypeOf(&HAProxyProvider{})))

	})

	It("Should connect to the backend", func() {
		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
		Expect(err).To(BeNil())
		err = createdBackend.Provider.Connect()
		Expect(err).To(BeNil())
		// fmt.Fprintf(GinkgoWriter, "URL: %s", httpurl)
		// fmt.Fprintf(GinkgoWriter, "DATA: %s", httpdata)
	})

	// Context("when handling load balancer monitors", func() {
	// 	It("Should get a monitor", func() {
	// 		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.Connect()
	// 		Expect(err).To(BeNil())
	// 		_, err = createdBackend.Provider.GetMonitor(monitor)
	// 		Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor/test-monitor"))
	// 		Expect(httpop).To(Equal("GET"))
	// 		Expect(err).To(BeNil())
	// 	})

	// 	It("Should create a monitor", func() {
	// 		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.Connect()
	// 		Expect(err).To(BeNil())
	// 		m, err := createdBackend.Provider.CreateMonitor(monitor)
	// 		Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor?idempotent=yes"))
	// 		Expect(httpop).To(Equal("POST"))

	// 		// <map[string]interface {} | len:1>: {
	// 		// 	"lbmonitor": <map[string]interface {} | len:7>{
	// 		// 		"destport": <float64>80,
	// 		// 		"downtime": <float64>16,
	// 		// 		"httprequest": <string>"GET /health",
	// 		// 		"interval": <float64>5,
	// 		// 		"monitorname": <string>"test-monitor",
	// 		// 		"respcode": <string>"",
	// 		// 		"type": <string>"HTTP",
	// 		// 	},
	// 		// }

	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["destport"]).To(Equal(float64(80)))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["httprequest"]).To(Equal("GET /health"))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["monitorname"]).To(Equal("test-monitor"))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["type"]).To(Equal("HTTP"))
	// 		Expect(err).To(BeNil())
	// 		Expect(m).NotTo(BeNil())
	// 	})

	// 	It("Should delete the monitor", func() {
	// 		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.Connect()
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.DeleteMonitor(monitor)
	// 		Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor/test-monitor?args=monitorname:test-monitor,type:http"))
	// 		Expect(httpop).To(Equal("DELETE"))
	// 		Expect(err).To(BeNil())
	// 	})

	// 	It("Should edit the monitor", func() {
	// 		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.Connect()
	// 		Expect(err).To(BeNil())
	// 		err = createdBackend.Provider.EditMonitor(monitor)
	// 		Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor?idempotent=yes"))
	// 		Expect(httpop).To(Equal("POST"))
	// 		// <map[string]interface {} | len:1>: {
	// 		// 	"lbmonitor": <map[string]interface {} | len:7>{
	// 		// 		"destport": <float64>80,
	// 		// 		"downtime": <float64>16,
	// 		// 		"httprequest": <string>"GET /health",
	// 		// 		"interval": <float64>5,
	// 		// 		"monitorname": <string>"test-monitor",
	// 		// 		"respcode": <string>"",
	// 		// 		"type": <string>"HTTP",
	// 		// 	},
	// 		// }
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["destport"]).To(Equal(float64(80)))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["httprequest"]).To(Equal("GET /health"))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["monitorname"]).To(Equal("test-monitor"))
	// 		Expect(httpdata["lbmonitor"].(map[string]interface{})["type"]).To(Equal("HTTP"))
	// 		Expect(err).To(BeNil())
	// 	})
	// })

	Context("when handling load balancer pools", func() {
		It("Should get a pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).To(BeNil())
			_, _ = createdBackend.Provider.GetPool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/v2/services/haproxy/configuration/backends/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
			// Our mock returns an empty map, so we can't check for equality
			// Expect(err).To(BeNil())
		})

		It("Should create a pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).To(BeNil())

			err = createdBackend.Provider.CreatePool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/v2/services/haproxy/configuration/backends"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("POST"))
			//   <map[string]interface {} | len:1>: {
			//       "name": <string>"test-pool",
			//   }
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-pool"))
			// We get an error because the lib expects a specific return and our mock server don't do this.
			// Lets just check the status code.
			//   s: "error creating pool(ERR) test-pool: unexpected success response: content available as default response in error (status 200): '[POST /services/haproxy/configuration/backends][200] createBackend default  &{Code:<nil> Message:<nil> Error:map[resp:OK]}'",
			Expect(err).To(MatchError(MatchRegexp("status 200")))
			// Expect(m).NotTo(BeNil())
		})

		It("Should delete the pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).To(BeNil())

			err = createdBackend.Provider.DeletePool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/v2/services/haproxy/configuration/backends/test-pool"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
			Expect(err).To(MatchError(MatchRegexp("status 200")))
			// Expect(err).To(BeNil())
		})

		It("Should edit the pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).To(BeNil())

			err = createdBackend.Provider.EditPool(pool)
			Eventually(httpdata.url, timeout, interval).Should(Equal("/v2/services/haproxy/configuration/backends"))
			Eventually(httpdata.method, timeout, interval).Should(Equal("POST"))
			Expect(err).To(MatchError(MatchRegexp("status 200")))
			Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-pool"))
			// Expect(err).To(BeNil())
		})
	})

	// 	Context("when handling load balancer VIPs", func() {
	// 		It("Should get a VIP", func() {
	// 			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 			Expect(err).NotTo(HaveOccurred())
	// 			err = createdBackend.Provider.Connect()
	// 			Expect(err).NotTo(HaveOccurred())

	// 			_, _ = createdBackend.Provider.GetVIP(VIP)
	// 			Expect(httpurl).To(Equal("/nitro/v1/config/lbvserver/test-vip"))
	// 			Expect(httpop).To(Equal("GET"))
	// 			Expect(err).NotTo(HaveOccurred())
	// 		})

	// 		It("Should create a VIP", func() {
	// 			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 			Expect(err).NotTo(HaveOccurred())
	// 			err = createdBackend.Provider.Connect()
	// 			Expect(err).NotTo(HaveOccurred())

	// 			m, err := createdBackend.Provider.CreateVIP(VIP)
	// 			Expect(httpurl).To(Equal("/nitro/v1/config/lbvserver_servicegroup_binding"))
	// 			Expect(httpop).To(Equal("POST"))

	// 			// <map[string]interface {} | len:5>: {
	// 			// 	"lbmonitor": <map[string]interface {} | len:6>{
	// 			// 		"interval": <float64>5,
	// 			// 		"downtime": <float64>16,
	// 			// 		"destport": <float64>80,
	// 			// 		"monitorname": <string>"test-monitor",
	// 			// 		"type": <string>"HTTP",
	// 			// 		"httprequest": <string>"GET /health",
	// 			// 	},
	// 			// 	"servicegroup": <map[string]interface {} | len:2>{
	// 			// 		"servicegroupname": <string>"test-pool",
	// 			// 		"servicetype": <string>"TCP",
	// 			// 	},
	// 			// 	"servicegroup_lbmonitor_binding": <map[string]interface {} | len:1>{
	// 			// 		"servicegroupname": <string>"test-pool",
	// 			// 	},
	// 			// 	"lbvserver": <map[string]interface {} | len:4>{
	// 			// 		"name": <string>"test-vip",
	// 			// 		"servicetype": <string>"TCP",
	// 			// 		"ipv46": <string>"1.2.3.4",
	// 			// 		"lbmethod": <string>"ROUNDROBIN",
	// 			// 	},
	// 			// 	"lbvserver_servicegroup_binding": <map[string]interface {} | len:2>{
	// 			// 		"name": <string>"test-vip",
	// 			// 		"servicegroupname": <string>"test-pool",
	// 			// 	},
	// 			// }

	// 			// Expect(httpdata).To(Equal(""))
	// 			Expect(httpdata["lbvserver"].(map[string]interface{})["name"]).To(Equal("test-vip"))
	// 			Expect(httpdata["lbvserver"].(map[string]interface{})["ipv46"]).To(Equal("1.2.3.4"))
	// 			Expect(err).NotTo(HaveOccurred())
	// 			Expect(m).NotTo(BeNil())
	// 		})

	// 		It("Should delete the VIP", func() {
	// 			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 			Expect(err).NotTo(HaveOccurred())
	// 			err = createdBackend.Provider.Connect()
	// 			Expect(err).NotTo(HaveOccurred())

	// 			err = createdBackend.Provider.DeleteVIP(VIP)
	// 			Expect(httpurl).To(Equal("/nitro/v1/config/lbvserver/test-vip"))
	// 			Expect(httpop).To(Equal("DELETE"))
	// 			Expect(err).NotTo(HaveOccurred())
	// 		})

	// 		It("Should edit the VIP", func() {
	// 			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
	// 			Expect(err).NotTo(HaveOccurred())
	// 			err = createdBackend.Provider.Connect()
	// 			Expect(err).NotTo(HaveOccurred())

	// 			err = createdBackend.Provider.EditVIP(VIP)
	// 			Expect(httpurl).To(Equal("/nitro/v1/config/lbvserver_servicegroup_binding"))
	// 			Expect(httpop).To(Equal("POST"))
	// 			Expect(httpdata["lbvserver"].(map[string]interface{})["name"]).To(Equal("test-vip"))
	// 			Expect(httpdata["lbvserver"].(map[string]interface{})["ipv46"]).To(Equal("1.2.3.4"))
	// 			Expect(err).NotTo(HaveOccurred())
	// 		})
	// })
	// })
})
