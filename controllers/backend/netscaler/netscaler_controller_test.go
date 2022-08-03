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
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/controllers/backend/netscaler"
)

func TestNetscaler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Netscaler Backend Suite")
}

var _ = Describe("Controllers/Backend/netscaler/netscaler_controller", func() {

	Context("When using a netscaler backend", func() {
		var ctx = context.TODO()
		var falseVar bool = false
		backend := &lbv1.LoadBalancerBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "netscaler-backend",
				Namespace: "default",
			},
			Spec: lbv1.LoadBalancerBackendSpec{
				Provider: lbv1.Provider{
					Vendor:        "netscaler",
					Host:          "1.2.3.4",
					Port:          443,
					Creds:         "creds-secret",
					Partition:     "Common",
					ValidateCerts: &falseVar,
				},
			},
		}

		It("Should create the backend", func() {
			createdBackend, err := Create(ctx, *backend, "username", "password")
			Expect(err).To(BeNil())
			Expect(createdBackend).NotTo(BeNil())
			Expect(reflect.TypeOf(createdBackend)).To(Equal(reflect.TypeOf(&NetscalerProvider{})))

		})
	})
})
