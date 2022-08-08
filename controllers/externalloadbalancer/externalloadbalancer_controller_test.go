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
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

var metricsPort = "38081"
var loadBalancer *lbv1.ExternalLoadBalancer
var credsSecret *corev1.Secret
var node *corev1.Node
var nodeList corev1.NodeList
var loadBalancerLookupKey types.NamespacedName
var nodeAddresses []string = []string{}

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

	Context("When managing an external load balancer", Ordered, func() {
		BeforeAll(func() {
			By("By creating a new Secret")
			ctx := context.Background()

			// Create the backend Secret
			credsSecret = &corev1.Secret{
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

			By("By creating a new ExternalLoadBalancer")
			loadBalancer = &lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-load-balancer",
					Namespace: Namespace,
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
						Vendor: "dummy",
						Host:   "1.2.3.4",
						Port:   443,
						Creds:  credsSecret.Name,
					},
				},
			}
		})

		It("should create a new ExternalLoadBalancer", func() {
			Expect(k8sClient.Create(ctx, loadBalancer)).Should(Succeed())
			Expect(loadBalancer.Spec.Provider.Vendor).Should(Equal("dummy"))

		})

		It("should check ExternalLoadBalancer metric is 1", func() {
			// Check metrics
			metricsBody := getMetricsBody(metricsPort)
			Expect(metricsBody).To(ContainSubstring("externallb_total 1"))
		})

		It("should create a node to be managed", func() {
			By("By checking the ExternalLoadBalancer has zero Nodes")
			loadBalancerLookupKey = types.NamespacedName{Name: loadBalancer.Name, Namespace: Namespace}
			Consistently(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return -1, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, duration, interval).Should(Equal(0))

			err := k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(nodeList.Items)).Should(Equal(0))

			By("By creating a Master Node")
			node := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			err = k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(nodeList.Items)).Should(Equal(1))

			By("By checking the ExternalLoadBalancer has one Node")
			Eventually(func() (int, error) {
				err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
				if err != nil {
					return 0, err
				}
				return len(loadBalancer.Status.Nodes), nil
			}, timeout, interval).Should(Equal(1))

			Expect(loadBalancer.Status.Provider.Vendor).Should(Equal("dummy"))

			for _, node := range loadBalancer.Status.Nodes {
				nodeAddresses = append(nodeAddresses, node.Host)
			}
			Expect(nodeAddresses).Should(ContainElement("1.1.1.1"))

			By("By checking the ExternalLoadBalancer metric instance has 1 node")
			metricsBody := getMetricsBody(metricsPort)
			metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 1)
			Expect(metricsBody).To(ContainSubstring(metricsOutput))

		})

		It("should create node not managed by this load balancer", func() {
			By("By creating a Worker Node")
			node := createReadyNode("infra-node-1", map[string]string{"node-role.kubernetes.io/infra": ""}, "1.1.1.5")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			err := k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(nodeList.Items)).Should(Equal(2))

			By("By checking the ExternalLoadBalancer still has one Node")
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
			Expect(nodeAddresses).Should(ContainElement("1.1.1.1"))

			By("By checking the ExternalLoadBalancer metric instance still has 1 node")
			metricsBody := getMetricsBody(metricsPort)
			metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 1)
			Expect(metricsBody).To(ContainSubstring(metricsOutput))
		})

		It("should create node not managed by this load balancer", func() {
			By("By creating an additional Master Node")
			node = createReadyNode("master-node-2", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.2")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			err := k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
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
			Expect(nodeAddresses).Should(ContainElements("1.1.1.1", "1.1.1.2"))

			By("By checking the ExternalLoadBalancer metric instance has 2 nodes")
			metricsBody := getMetricsBody(metricsPort)
			metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 2)
			Expect(metricsBody).To(ContainSubstring(metricsOutput))
		})

		It("should create a master node not that is not ready", func() {
			By("By creating an additional Master Node that is not ready")
			node = createNotReadyNode("master-node-3", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.3")

			Expect(k8sClient.Create(ctx, node)).Should(Succeed())

			err := k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
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

			By("By checking the ExternalLoadBalancer metric instance still has 2 nodes")
			metricsBody := getMetricsBody(metricsPort)
			metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 2)
			Expect(metricsBody).To(ContainSubstring(metricsOutput))
		})

		It("should remove a master node from load balancer instance", func() {
			By("By removing one Master Node")
			k8sClient.Get(ctx, types.NamespacedName{Name: "master-node-1"}, node)
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())

			err := k8sClient.List(ctx, &nodeList)
			Expect(err).NotTo(HaveOccurred())
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

			By("By checking the ExternalLoadBalancer metric instance has 1 node")
			metricsBody := getMetricsBody(metricsPort)
			metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 1)
			Expect(metricsBody).To(ContainSubstring(metricsOutput))
		})
	})
})
