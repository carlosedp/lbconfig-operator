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

package dummy_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/controller"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/dummy"
)

func TestDummy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dummy Backend Suite")
}

var _ = Describe("Controllers/Backend/dummy/dummy_controller", func() {

	Context("When using a dummy backend", func() {
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
				Name:      "dummy-backend",
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
					Vendor: "Dummy",
					Host:   "1.2.3.4",
					Port:   443,
					Creds:  credsSecret.Name,
				},
			},
		}

		It("Should create the backend", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).To(BeNil())
			Expect(createdBackend).NotTo(BeNil())
			Expect(ListProviders()).To(ContainElement(strings.ToLower("dummy")))
			Expect(reflect.TypeOf(createdBackend.Provider)).To(Equal(reflect.TypeOf(&DummyProvider{})))
		})
	})
})
