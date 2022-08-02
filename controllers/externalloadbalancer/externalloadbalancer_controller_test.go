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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

var _ = Describe("ExternalLoadBalancer controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		// 	JobName          = "test-job"
		SecretName = "backend-creds"
		Namespace  = "default"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When using auxiliary funtions", func() {
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
			By("Checking if labels are correct")
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
	})

	Context("When managing an external load balancer", func() {
		It("Should use secret and dummy backend as load balancer", func() {

			By("By creating a new Secret")
			ctx := context.Background()

			// Create the backend Secret
			credsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: Namespace,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpassword"),
				},
			}
			Expect(k8sClient.Create(ctx, credsSecret)).Should(Succeed())

			secretLookupKey := types.NamespacedName{Name: SecretName, Namespace: Namespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, credsSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(credsSecret.Data["username"]).Should(Equal([]byte("testuser")))

			// Create the dummy backend
			By("By creating a new LoadBalancer backend")
			backend := &lbv1.LoadBalancerBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-backend",
					Namespace: Namespace,
				},
				Spec: lbv1.LoadBalancerBackendSpec{
					Provider: lbv1.Provider{
						Vendor: "dummy",
						Host:   "1.2.3.4",
						Port:   443,
						Creds:  credsSecret.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, backend)).Should(Succeed())
			backendLookupKey := types.NamespacedName{Name: backend.Name, Namespace: Namespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backendLookupKey, backend)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(backend.Spec.Provider.Vendor).Should(Equal("dummy"))
			// Fixthis
			// fmt.Fprintf(GinkgoWriter, "Backend Vendor: %v\n", backend.Status.Provider.Vendor)
			// Expect(backend.Status.Provider.Vendor).Should(Equal("dummy"))

			// Create the ExternalLoadBalancer
			By("By creating a new ExternalLoadBalancer")
			loadBalancer := &lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-load-balancer",
					Namespace: Namespace,
				},
				Spec: lbv1.ExternalLoadBalancerSpec{
					Vip:     "10.0.0.1",
					Type:    "master",
					Backend: backend.Name,
					Ports:   []int{443},
					Monitor: lbv1.Monitor{
						Path:        "/",
						Port:        80,
						MonitorType: "http",
					},
				},
			}
			Expect(k8sClient.Create(ctx, loadBalancer)).Should(Succeed())

			By("By checking the ExternalLoadBalancer has zero Nodes")
			loadBalancerLookupKey := types.NamespacedName{Name: loadBalancer.Name, Namespace: Namespace}
			Consistently(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return -1, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, duration, interval).Should(Equal(0))

			var nodeList corev1.NodeList
			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}

			Expect(len(nodeList.Items)).Should(Equal(0))

			By("By creating a Master Node")
			node := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}
			Expect(len(nodeList.Items)).Should(Equal(1))

			By("By checking the ExternalLoadBalancer has one Node")

			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(1))

			Expect(loadBalancer.Status.Nodes[len(loadBalancer.Status.Nodes)-1].Host).Should(Equal("1.1.1.1"))

			By("By creating a Worker Node")
			node = createReadyNode("infra-node-1", map[string]string{"node-role.kubernetes.io/infra": ""}, "1.1.1.5")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}
			Expect(len(nodeList.Items)).Should(Equal(2))

			By("By checking the ExternalLoadBalancer still has one Node")

			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(1))

			var nodeAddresses []string = []string{}
			for _, node := range loadBalancer.Status.Nodes {
				nodeAddresses = append(nodeAddresses, node.Host)
			}
			Expect(nodeAddresses).Should(ContainElement("1.1.1.1"))

			By("By creating an additional Master Node")
			node = createReadyNode("master-node-2", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.2")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}
			Expect(len(nodeList.Items)).Should(Equal(3))

			By("By checking the ExternalLoadBalancer has two Nodes")

			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(2))

			nodeAddresses = []string{}
			for _, node := range loadBalancer.Status.Nodes {
				nodeAddresses = append(nodeAddresses, node.Host)
			}
			Expect(nodeAddresses).Should(ContainElement("1.1.1.2"))

			By("By creating an additional Master Node that is not ready")
			node = createNotReadyNode("master-node-3", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.3")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}
			Expect(len(nodeList.Items)).Should(Equal(4))

			By("By checking the ExternalLoadBalancer still has two Nodes")

			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(2))

			nodeAddresses = []string{}
			for _, node := range loadBalancer.Status.Nodes {
				nodeAddresses = append(nodeAddresses, node.Host)
			}
			Expect(nodeAddresses).ShouldNot(ContainElement("1.1.1.3"))

			By("By removing one Master Node")
			k8sClient.Get(ctx, types.NamespacedName{Name: "master-node-1"}, node)
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())

			if err := k8sClient.List(ctx, &nodeList); err != nil {
				return
			}
			Expect(len(nodeList.Items)).Should(Equal(3))

			By("By checking the ExternalLoadBalancer has one Node")

			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(1))

			nodeAddresses = []string{}
			for _, node := range loadBalancer.Status.Nodes {
				nodeAddresses = append(nodeAddresses, node.Host)
			}
			Expect(nodeAddresses).ShouldNot(ContainElement("1.1.1.1"))

		})
	})
})
