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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/apis/externalloadbalancer/v1"
)

var _ = Describe("ExternalLoadBalancer controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		// 	JobName          = "test-job"
		SecretName = "backend-creds"
		Namespace  = "default"
	)

	Context("When using utility funtions", func() {
		It("Should check contain labels", func() {
			labels := map[string]string{"label1": "value1", "label2": "value2"}
			containedLabels := map[string]string{"label1": "value1"}
			notContainedLabels := map[string]string{"label3": "value3"}

			By("Checking if valid labels are contained")
			Expect(containsLabels(labels, containedLabels)).To(BeTrue())

			By("Checking if invalid labels are not contained")
			Expect(containsLabels(labels, notContainedLabels)).To(BeFalse())
		})

		It("Should compute labels from LoadBalancer instance", func() {
			loadBalancer := lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-load-balancer",
					Namespace: Namespace,
				},
				Spec: lbv1.ExternalLoadBalancerSpec{
					Type: "master",
				},
			}
			Expect(computeLabels(loadBalancer)).To(Equal(map[string]string{"node-role.kubernetes.io/master": ""}))
		})

		It("Should compute custom labels from LoadBalancer instance", func() {
			loadBalancer := lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-load-balancer",
					Namespace: Namespace,
				},
				Spec: lbv1.ExternalLoadBalancerSpec{
					NodeLabels: map[string]string{"node-role.kubernetes.io/custom": ""},
				},
			}
			Expect(computeLabels(loadBalancer)).To(Equal(map[string]string{"node-role.kubernetes.io/custom": ""}))
		})

		It("Should check if nodes changed conditions", func() {
			n1 := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")
			n2 := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")
			n3 := createNotReadyNode("master-node-2", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.2")

			Expect(hasNodeChanged(n1, n2)).To(BeFalse())
			Expect(hasNodeChanged(n1, n3)).To(BeTrue())
		})

		It("Should check if nodes changed IP addresses", func() {
			n1 := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")
			n2 := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")
			n3 := createReadyNode("master-node-2", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.2")

			Expect(hasNodeChanged(n1, n2)).To(BeFalse())
			Expect(hasNodeChanged(n1, n3)).To(BeTrue())
		})
	})
})
