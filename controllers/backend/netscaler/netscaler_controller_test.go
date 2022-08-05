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

package netscaler_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/netscaler"
)

func TestNetscaler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Netscaler Backend Suite")
}

var HTTP_PORT = rand.Intn(65000-35000) + 35000
var httpurl string
var httpop string
var httppost = make(map[string][]string)
var httpdata map[string]interface{}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
	httpurl = r.URL.String()
	httpop = r.Method
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&httpdata)
	r.ParseForm()

	for k, v := range r.Form {
		httppost[k] = v
	}
}

var _ = BeforeSuite(func() {
	// Lets start a local http server to answer the API calls
	go func() {
		rtr := mux.NewRouter()
		rtr.HandleFunc("/{url:.*}", pageHandler)
		http.Handle("/", rtr)
		err := http.ListenAndServe((":" + strconv.Itoa(HTTP_PORT)), nil)
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

var _ = Describe("Controllers/Backend/netscaler/netscaler_controller", func() {

	Context("When using a netscaler backend", func() {
		var ctx = context.TODO()
		var falseVar bool = false

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
				Name:      "netscaler-backend",
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
					Vendor:        "netscaler",
					Host:          "127.0.0.1",
					Port:          HTTP_PORT,
					Creds:         credsSecret.Name,
					Partition:     "Common",
					ValidateCerts: &falseVar,
				},
			},
		}

		It("Should create the backend", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			Expect(createdBackend).NotTo(BeNil())
			Expect(ListProviders()).To(ContainElement("netscaler"))
			Expect(reflect.TypeOf(createdBackend.Provider)).To(Equal(reflect.TypeOf(&NetscalerProvider{})))

		})

		It("Should connect to the backend", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			err = createdBackend.Provider.Connect()
			Expect(err).To(BeNil())
		})

		Context("when handling load balancer monitors", func() {
			It("Should get a monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())
				_, err = createdBackend.Provider.GetMonitor(monitor)
				Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor/test-monitor"))
				Expect(httpop).To(Equal("GET"))
				Expect(err).To(BeNil())
			})

			It("Should create a monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())
				m, err := createdBackend.Provider.CreateMonitor(monitor)
				Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor?idempotent=yes"))
				Expect(httpop).To(Equal("POST"))

				// <map[string]interface {} | len:1>: {
				// 	"lbmonitor": <map[string]interface {} | len:7>{
				// 		"destport": <float64>80,
				// 		"downtime": <float64>16,
				// 		"httprequest": <string>"GET /health",
				// 		"interval": <float64>5,
				// 		"monitorname": <string>"test-monitor",
				// 		"respcode": <string>"",
				// 		"type": <string>"HTTP",
				// 	},
				// }

				Expect(httpdata["lbmonitor"].(map[string]interface{})["destport"]).To(Equal(float64(80)))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["httprequest"]).To(Equal("GET /health"))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["monitorname"]).To(Equal("test-monitor"))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["type"]).To(Equal("HTTP"))
				Expect(err).To(BeNil())
				Expect(m).NotTo(BeNil())
			})

			It("Should delete the monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())
				err = createdBackend.Provider.DeleteMonitor(monitor)
				Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor/test-monitor?args=monitorname:test-monitor,type:http"))
				Expect(httpop).To(Equal("DELETE"))
				Expect(err).To(BeNil())
			})

			It("Should edit the monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())
				err = createdBackend.Provider.EditMonitor(monitor)
				Expect(httpurl).To(Equal("/nitro/v1/config/lbmonitor?idempotent=yes"))
				Expect(httpop).To(Equal("POST"))
				// <map[string]interface {} | len:1>: {
				// 	"lbmonitor": <map[string]interface {} | len:7>{
				// 		"destport": <float64>80,
				// 		"downtime": <float64>16,
				// 		"httprequest": <string>"GET /health",
				// 		"interval": <float64>5,
				// 		"monitorname": <string>"test-monitor",
				// 		"respcode": <string>"",
				// 		"type": <string>"HTTP",
				// 	},
				// }
				Expect(httpdata["lbmonitor"].(map[string]interface{})["destport"]).To(Equal(float64(80)))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["httprequest"]).To(Equal("GET /health"))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["monitorname"]).To(Equal("test-monitor"))
				Expect(httpdata["lbmonitor"].(map[string]interface{})["type"]).To(Equal("HTTP"))
				Expect(err).To(BeNil())
			})
		})

		Context("when handling load balancer pools", func() {
			It("Should get a pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())

				_, err = createdBackend.Provider.GetPool(pool)
				Expect(httpurl).To(Equal("/nitro/v1/config/servicegroup/test-pool"))
				Expect(httpop).To(Equal("GET"))
				Expect(err).To(BeNil())
			})

			It("Should create a pool", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())

				m, err := createdBackend.Provider.CreatePool(pool)
				Expect(httpurl).To(Equal("/nitro/v1/config/servicegroup_lbmonitor_binding"))
				Expect(httpop).To(Equal("POST"))
				// <map[string]interface {} | len:1>: {
				// 	"servicegroupname": <string>"test-pool",
				// }
				// Expect(httpdata["servicegroup_lbmonitor_binding"]).To(Equal(""))
				Expect(httpdata["servicegroup_lbmonitor_binding"].(map[string]interface{})["servicegroupname"]).To(Equal("test-pool"))
				Expect(err).To(BeNil())
				Expect(m).NotTo(BeNil())
			})

			It("Should delete the monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())

				err = createdBackend.Provider.DeletePool(pool)
				Expect(httpurl).To(Equal("/nitro/v1/config/servicegroup/test-pool"))
				Expect(httpop).To(Equal("DELETE"))
				Expect(err).To(BeNil())
			})

			It("Should edit the monitor", func() {
				createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
				Expect(err).To(BeNil())
				err = createdBackend.Provider.Connect()
				Expect(err).To(BeNil())

				err = createdBackend.Provider.EditPool(pool)
				Expect(httpurl).To(Equal("/nitro/v1/config/servicegroup_lbmonitor_binding"))
				Expect(httpop).To(Equal("POST"))
				// <map[string]interface {} | len:1>: {
				// 	"servicegroupname": <string>"test-pool",
				// }
				// Expect(httpdata["servicegroup_lbmonitor_binding"]).To(Equal(""))
				Expect(httpdata["servicegroup_lbmonitor_binding"].(map[string]interface{})["servicegroupname"]).To(Equal("test-pool"))
				Expect(err).To(BeNil())
			})
		})
	})
})
