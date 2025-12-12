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
	"fmt"
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

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_controller"
	. "github.com/carlosedp/lbconfig-operator/internal/controller/backend/haproxy"
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
			ValidateCerts: false,
			Debug:         false,
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

var VIP = &lbv1.VIP{
	Name: "test-vip",
	Pool: pool.Name,
	IP:   "1.2.3.4",
}

// Store the http session data for the request
type httpdataStruct struct {
	url    []string
	method []string
	data   []string
	post   map[string][]string
}

var _ = Describe("When using a HAProxy backend", func() {

	var server *httptest.Server
	var httpdata httpdataStruct
	var ctx = context.TODO()

	BeforeEach(func() {
		httpdata = httpdataStruct{}
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(GinkgoWriter, "Received a request for %s, method %s\n", r.URL.String(), r.Method)
			httpdata.url = append(httpdata.url, r.URL.String())
			httpdata.method = append(httpdata.method, r.Method)
			body, _ := io.ReadAll(r.Body)
			httpdata.data = append(httpdata.data, string(body))
			for k, v := range r.Form {
				httpdata.post[k] = v
			}
			w.WriteHeader(200)
			// w.Write([]byte("{'resp': 'ok'}"))

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
		Expect(err).To(BeNil())
		Expect(createdBackend).NotTo(BeNil())
		Expect(ListProviders()).To(ContainElement(strings.ToLower("HAProxy")))
		Expect(reflect.TypeOf(createdBackend.Provider)).To(Equal(reflect.TypeOf(&HAProxyProvider{})))

	})

	It("Should connect to the backend", func() {
		createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
		Expect(err).To(BeNil())
		_ = createdBackend.Provider.Connect()
		// Expect(err).To(BeNil())
		// fmt.Fprintf(GinkgoWriter, "URL: %s", httpurl)
		// fmt.Fprintf(GinkgoWriter, "DATA: %s", httpdata)
	})

	Context("when managing HAProxy", func() {
		var createdBackend *BackendController
		var err error
		BeforeEach(func() {
			createdBackend, err = CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			_ = createdBackend.Provider.Connect()
		})

		Context("when handling load balancer monitors", func() {
			It("Should get a monitor", func() {
				_, err = createdBackend.Provider.GetMonitor(monitor)
				Expect(err).To(BeNil())
			})

			It("Should create a monitor", func() {
				err := createdBackend.Provider.CreateMonitor(monitor)
				Expect(err).To(BeNil())
			})

			It("Should delete the monitor", func() {
				err = createdBackend.Provider.DeleteMonitor(monitor)
				Expect(err).To(BeNil())
			})

			It("Should edit the monitor", func() {
				err = createdBackend.Provider.EditMonitor(monitor)
				Expect(err).To(BeNil())
			})
		})

		Context("when handling load balancer pools", func() {
			// It("Should get a pool", func() {
			// 	_, _ = createdBackend.Provider.GetPool(pool)
			// 	url := "/v2/services/haproxy/configuration/backends/test-pool"
			// 	Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
			// 	i := indexOf(url, httpdata.url)
			// 	Eventually(httpdata.method[i], timeout, interval).Should(Equal("GET"))
			// 	// Our mock returns an empty map, so we can't check for equality
			// 	// Expect(err).To(BeNil())
			// })

			It("Should create a pool", func() {
				err = createdBackend.Provider.CreatePool(pool)
				url := "/v2/services/haproxy/configuration/backends"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("POST"))
				//   <map[string]interface {} | len:1>: {
				//       "name": <string>"test-pool",
				//   }
				Eventually(gjson.Get(httpdata.data[i], "name").String(), timeout, interval).Should(Equal("test-pool"))
				// We get an error because the lib expects a specific return and our mock server don't do this.
				// Lets just check the status code.
				//   s: "error creating pool(ERR) test-pool: unexpected success response: content available as default response in error (status 200): '[POST /services/haproxy/configuration/backends][200] createBackend default  &{Code:<nil> Message:<nil> Error:map[resp:OK]}'",
				// Expect(err).To(MatchError(MatchRegexp("status 200")))
				// Expect(m).NotTo(BeNil())
			})

			It("Should delete the pool", func() {
				err = createdBackend.Provider.DeletePool(pool)
				url := "/v2/services/haproxy/configuration/backends/test-pool"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("DELETE"))
				Expect(err).To(MatchError(MatchRegexp("status 200")))
				// Expect(err).To(BeNil())
			})

			It("Should edit the pool", func() {
				_ = createdBackend.Provider.EditPool(pool)
				url := "/v2/services/haproxy/configuration/backends/test-pool"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("PUT"))

				// Expect(err).To(MatchError(MatchRegexp("status 200")))
				Eventually(gjson.Get(httpdata.data[i], "name").String(), timeout, interval).Should(Equal("test-pool"))
				// Expect(err).To(BeNil())
			})
		})

		Context("when handling load balancer VIPs", func() {
			// It("Should get a VIP", func() {
			// 	_, _ = createdBackend.Provider.GetVIP(VIP)

			// 	url := "/v2/services/haproxy/configuration/frontends/test-vip"
			// 	Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
			// 	i := indexOf(url, httpdata.url)
			// 	Eventually(httpdata.method[i], timeout, interval).Should(Equal("GET"))
			// })

			It("Should create a VIP", func() {
				err := createdBackend.Provider.CreateVIP(VIP)
				url := "/v2/services/haproxy/configuration/frontends"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("POST"))
				//   {"default_backend":"test-pool","mode":"tcp","name":"test-vip"}
				Eventually(gjson.Get(httpdata.data[i], "default_backend").String(), timeout, interval).Should(Equal("test-pool"))
				Eventually(gjson.Get(httpdata.data[i], "mode").String(), timeout, interval).Should(Equal("tcp"))
				Eventually(gjson.Get(httpdata.data[i], "name").String(), timeout, interval).Should(Equal("test-vip"))
				// We get an error because the lib expects a specific return and our mock server don't do this.
				// Lets just check the status code.
				//   s: "error creating pool(ERR) test-pool: unexpected success response: content available as default response in error (status 200): '[POST /services/haproxy/configuration/backends][200] createBackend default  &{Code:<nil> Message:<nil> Error:map[resp:OK]}'",
				Expect(err).To(MatchError(MatchRegexp("status 200")))
			})

			It("Should delete the VIP", func() {
				err = createdBackend.Provider.DeleteVIP(VIP)
				url := "/v2/services/haproxy/configuration/frontends/test-vip"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("DELETE"))
				Expect(err).To(MatchError(MatchRegexp("status 200")))
			})

			It("Should edit the VIP", func() {
				err = createdBackend.Provider.EditVIP(VIP)
				err = createdBackend.Provider.DeleteVIP(VIP)
				url := "/v2/services/haproxy/configuration/frontends/"
				Eventually(httpdata.url, timeout, interval).Should(ContainElement(url))
				i := indexOf(url, httpdata.url)
				Eventually(httpdata.method[i], timeout, interval).Should(Equal("PUT"))
				Eventually(gjson.Get(httpdata.data[i], "name").String(), timeout, interval).Should(Equal("test-vip"))
				Eventually(gjson.Get(httpdata.data[i], "mode").String(), timeout, interval).Should(Equal("http"))
				Expect(err).To(MatchError(MatchRegexp("status 200")))
			})
		})
	})
})

func indexOf(element string, data []string) int {
	for k, v := range data {
		if element == v {
			return k
		}
	}
	return -1 //not found.
}
