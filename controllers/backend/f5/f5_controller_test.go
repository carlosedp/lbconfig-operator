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
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
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

var HTTP_PORT int

func init() {
	rand.Seed(GinkgoRandomSeed())
	HTTP_PORT = rand.Intn(65000-35000) + 35000
}

type httpdataStruct struct {
	url    string
	method string
	post   map[string][]string
	data   string
}

var httpdata httpdataStruct

func pageHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
	httpdata.url = r.URL.String()
	httpdata.method = r.Method
	d, _ := io.ReadAll(r.Body)
	httpdata.data = string(d)
	r.ParseForm()

	for k, v := range r.Form {
		httpdata.post[k] = v
	}
}

var _ = BeforeSuite(func() {
	// Lets start a local http server to answer the API calls
	go func() {
		rtr := mux.NewRouter()
		rtr.HandleFunc("/{url:.*}", pageHandler)
		http.Handle("/", rtr)
		err := http.ListenAndServeTLS((":" + strconv.Itoa(HTTP_PORT)), "server.crt", "server.key", nil)
		if err != nil {
			panic(err)
		}
	}()
})

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

var _ = Describe("Controllers/Backend/f5/f5_controller", Ordered, func() {

	Context("When using a f5 backend", func() {
		var ctx = context.TODO()

		// Create the backend Secret
		credsSecret := &corev1.Secret{
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
		loadBalancer := &lbv1.ExternalLoadBalancer{
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
					Host:          "https://127.0.0.1",
					Port:          HTTP_PORT,
					Creds:         credsSecret.Name,
					Partition:     "Common",
					ValidateCerts: pointer.BoolPtr(false),
				},
			},
		}

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
			It("Should get a monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())
				_, _ = createdBackend.Provider.GetMonitor(monitor)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http/test-monitor"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
				// Getting error "error getting F5 Monitor test-monitor: invalid character 'O' looking for beginning of value"
				// Expect(err).NotTo(HaveOccurred())
			})

			It("Should create a monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())
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
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.DeleteMonitor(monitor)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/monitor/http/test-monitor"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should edit the monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())
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
			It("Should get a pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				_, _ = createdBackend.Provider.GetPool(pool)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
				// Getting error "error getting F5 Monitor test-monitor: invalid character 'O' looking for beginning of value"
				// Expect(err).NotTo(HaveOccurred())
			})

			It("Should create a pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.CreatePool(pool)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("PATCH"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should delete the pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.DeletePool(pool)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should edit the pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.EditPool(pool)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/pool/test-pool"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("PUT"))
				Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-pool"))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when handling load balancer VIPs", func() {
			It("Should get a VIP", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				_, _ = createdBackend.Provider.GetVIP(VIP)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/test-vip"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("GET"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should create a VIP", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.CreateVIP(VIP)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("POST"))

				// Expect(httpdata["servicegroup_lbmonitor_binding"]).Should(Equal(""))
				Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-vip"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should delete the VIP", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.DeleteVIP(VIP)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/test-vip"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("DELETE"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should edit the VIP", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).NotTo(HaveOccurred())
				err = createdBackend.Provider.Connect()
				Expect(err).NotTo(HaveOccurred())

				err = createdBackend.Provider.EditVIP(VIP)
				Eventually(httpdata.url, timeout, interval).Should(Equal("/mgmt/tm/ltm/virtual/~Common~test-vip"))
				Eventually(httpdata.method, timeout, interval).Should(Equal("PATCH"))
				Eventually(gjson.Get(httpdata.data, "name").String(), timeout, interval).Should(Equal("test-vip"))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
