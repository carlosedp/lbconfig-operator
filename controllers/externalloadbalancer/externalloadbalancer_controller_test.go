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

	lbv1 "github.com/carlosedp/lbconfig-operator/apis/externalloadbalancer/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	SecretName = "backend-creds"
	Namespace  = "default"

	timeout  = time.Second * 10
	duration = time.Second * 10
	interval = time.Millisecond * 250
)

var node *corev1.Node
var nodeList corev1.NodeList
var loadBalancerLookupKey types.NamespacedName
var nodeAddresses []string = []string{}

var credsSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      SecretName,
		Namespace: Namespace,
	},
	Data: map[string][]byte{
		"username": []byte("testuser"),
		"password": []byte("testpassword"),
	},
}

var loadBalancer = &lbv1.ExternalLoadBalancer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-load-balancer",
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

var _ = Describe("ExternalLoadBalancer controller", Ordered, func() {
	ctx := context.Background()
	secretLookupKey := types.NamespacedName{Name: SecretName, Namespace: Namespace}
	loadBalancerLookupKey = types.NamespacedName{Name: loadBalancer.Name, Namespace: Namespace}

	// It("should return error creating a new ExternalLoadBalancer without a secret", func() {
	// 	// Modified the load balancer to create before having a secret.
	// 	lb2 := loadBalancer.DeepCopy()
	// 	lb2.ObjectMeta.Name = "test-load-balancer-err1"
	// 	// Check it was created
	// 	Expect(k8sClient.Create(ctx, lb2)).Should(Succeed())

	// 	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: lb2.Name, Namespace: Namespace}})
	// 	// Expect(err).NotTo(BeNil())
	// 	Eventually(err, timeout, interval).ShouldNot(BeNil())
	// 	// Eventually(err, timeout, interval).Should(MatchError(MatchRegexp("provider credentials secret not found")))

	// 	// Delete the created load balancer
	// 	Expect(k8sClient.Delete(ctx, lb2)).Should(Succeed())
	// 	Consistently(func() (int, error) {
	// 		lblist := &lbv1.ExternalLoadBalancerList{}
	// 		err := k8sClient.List(ctx, lblist)
	// 		if err != nil {
	// 			return -1, err
	// 		}
	// 		return len(lblist.Items), nil
	// 	}, duration, interval).Should(Equal(0))
	// })

	It("should create a backend secret", func() {
		Expect(k8sClient.Create(ctx, credsSecret)).Should(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, secretLookupKey, credsSecret)
			return err == nil
		}, timeout, interval).Should(BeTrue())
		Expect(credsSecret.Data["username"]).Should(Equal([]byte("testuser")))
	})

	It("should return error creating a new ExternalLoadBalancer without labels", func() {
		lb3 := loadBalancer.DeepCopy()
		lb3.ObjectMeta.Name = "test-load-balancer-err2"
		lb3.Spec.Type = ""
		Expect(k8sClient.Create(ctx, lb3)).Should(Succeed())
		Eventually(func() error {
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: lb3.Name, Namespace: Namespace}})
			return err
		}, timeout, interval).Should(MatchError(MatchRegexp("undefined loadbalancer type or no nodelabels defined")))

		Expect(k8sClient.Delete(ctx, lb3)).Should(Succeed())
		Consistently(func() (int, error) {
			lblist := &lbv1.ExternalLoadBalancerList{}
			err := k8sClient.List(ctx, lblist)
			if err != nil {
				return -1, err
			}
			return len(lblist.Items), nil
		}, duration, interval).Should(Equal(0))
	})

	It("should create a new ExternalLoadBalancer", func() {
		Expect(k8sClient.Create(ctx, loadBalancer)).Should(Succeed())
		Eventually(func() string {
			k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
			return loadBalancer.Status.Provider.Vendor
		}, timeout, interval).Should(Equal("Dummy"))
	})

	It("should check ExternalLoadBalancer metric is 1", func() {
		Expect(getMetricsBody(metricsPort)).To(ContainSubstring("externallb_total 1"))
	})

	It("should create a node to be managed", func() {
		By("By checking the ExternalLoadBalancer has zero Nodes")

		Consistently(func() (int, error) {
			err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
			if err != nil {
				return -1, err
			}
			return len(loadBalancer.Status.Nodes), nil
		}, duration, interval).Should(Equal(0))

		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
		Expect(len(nodeList.Items)).Should(Equal(0))

		By("By creating a Master Node")
		node := createReadyNode("master-node-1", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.1")

		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
		Expect(len(nodeList.Items)).Should(Equal(1))

		By("By checking the ExternalLoadBalancer has one Node")
		Eventually(func() (int, error) {
			err := k8sClient.Get(ctx, loadBalancerLookupKey, loadBalancer)
			if err != nil {
				return 0, err
			}
			return len(loadBalancer.Status.Nodes), nil
		}, timeout, interval).Should(Equal(1))

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
		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
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
		IPs := []corev1.NodeAddress{
			{
				Type:    corev1.NodeInternalIP,
				Address: "9.9.9.9",
			},
			{
				Type:    corev1.NodeExternalIP,
				Address: "1.1.1.2",
			},
		}
		node.Status.Addresses = IPs
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
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
		// Check that even though the node has External and Internal IPs, the External IP is used
		Expect(nodeAddresses).ShouldNot(ContainElement("9.9.9.9"))

		By("By checking the ExternalLoadBalancer metric instance has 2 nodes")
		metricsBody := getMetricsBody(metricsPort)
		metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 2)
		Expect(metricsBody).To(ContainSubstring(metricsOutput))
	})

	It("should create a master node not that is not ready", func() {
		By("By creating an additional Master Node that is not ready")
		node = createNotReadyNode("master-node-3", map[string]string{"node-role.kubernetes.io/master": ""}, "1.1.1.3")
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
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
		fmt.Fprintf(GinkgoWriter, "metricsBody: %s\n", metricsBody)
		metricsOutput := fmt.Sprintf(`externallb_nodes{backend_vendor="%s",name="%s",namespace="%s",port="%s",type="%s",vip="%s"} %d`, loadBalancer.Spec.Provider.Vendor, loadBalancer.Name, Namespace, strconv.Itoa(loadBalancer.Spec.Provider.Port), loadBalancer.Spec.Type, loadBalancer.Spec.Vip, 2)
		Expect(metricsBody).To(ContainSubstring(metricsOutput))
	})

	It("should remove a master node from load balancer instance", func() {
		By("By removing one Master Node")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "master-node-1"}, node)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, node)).Should(Succeed())
		Expect(k8sClient.List(ctx, &nodeList)).Should(Succeed())
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

	It("should delete the external load balancer instance", func() {
		By("By removing the instance")
		Expect(k8sClient.Delete(ctx, loadBalancer)).Should(Succeed())
		Eventually(func() (int, error) {
			lblist := &lbv1.ExternalLoadBalancerList{}
			err := k8sClient.List(ctx, lblist)
			if err != nil {
				return -1, err
			}
			return len(lblist.Items), nil
		}, timeout, interval).Should(Equal(0))
	})

	It("should check ExternalLoadBalancer metric is 0", func() {
		metricsBody := getMetricsBody(metricsPort)
		Expect(metricsBody).ToNot(ContainSubstring("externallb_nodes"))
		Expect(metricsBody).To(ContainSubstring("externallb_total 0"))
	})
})
