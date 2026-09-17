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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

const (
	drainLBName         = "drain-f5"
	drainCredsName      = "drain-f5-creds"
	drainPoolName       = "Pool-" + drainLBName + "-80"
	drainNodeLabel      = "lbconfig.carlosedp.com/drain-test"
	drainNodeLabelValue = "true"
	drainNode1Name      = "drain-node-1"
	drainNode2Name      = "drain-node-2"
	drainNode1IP        = "10.10.0.1"
	drainNode2IP        = "10.10.0.2"
	drainTimeoutSeconds = 2
)

func drainNodeLabels() map[string]string {
	return map[string]string{drainNodeLabel: drainNodeLabelValue}
}

// drainTestSpec builds a valid ExternalLoadBalancer spec for the given provider with draining enabled
func drainTestSpec(provider lbv1.Provider) lbv1.ExternalLoadBalancerSpec {
	return lbv1.ExternalLoadBalancerSpec{
		Vip:        "10.10.0.100",
		NodeLabels: drainNodeLabels(),
		Ports:      []int{80},
		Monitor:    lbv1.Monitor{Path: "/healthz", Port: 80, MonitorType: "http"},
		Provider:   provider,
		Drain:      &lbv1.DrainConfig{Enabled: true, TimeoutSeconds: drainTimeoutSeconds},
	}
}

var _ = Describe("ExternalLoadBalancer drain configuration", func() {
	ctx := context.Background()
	provider := lbv1.Provider{Vendor: "Dummy", Host: "1.2.3.4", Port: 443, Creds: SecretName}

	It("Should default the drain timeout to 30 seconds", func() {
		lb := &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: "drain-defaults", Namespace: Namespace},
			Spec:       drainTestSpec(provider),
		}
		lb.Spec.Drain = &lbv1.DrainConfig{Enabled: true}
		Expect(k8sClient.Create(ctx, lb, client.DryRunAll)).To(Succeed())
		Expect(lb.Spec.Drain.TimeoutSeconds).To(Equal(30))
	})

	DescribeTable("Should reject drain timeouts out of range",
		func(timeoutSeconds int64, message string) {
			lb := &lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{Name: "drain-invalid", Namespace: Namespace},
				Spec:       drainTestSpec(provider),
			}
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(lb)
			Expect(err).NotTo(HaveOccurred())
			delete(obj, "status")
			// Set through unstructured since the typed client omits a zero timeout
			Expect(unstructured.SetNestedField(obj, timeoutSeconds, "spec", "drain", "timeoutSeconds")).To(Succeed())
			u := &unstructured.Unstructured{Object: obj}
			u.SetGroupVersionKind(lbv1.GroupVersion.WithKind("ExternalLoadBalancer"))
			Expect(k8sClient.Create(ctx, u, client.DryRunAll)).To(MatchError(ContainSubstring(message)))
		},
		Entry("zero", int64(0), "should be greater than or equal to 1"),
		Entry("above one hour", int64(3601), "should be less than or equal to 3600"),
	)
})

var _ = Describe("ExternalLoadBalancer graceful connection draining with a F5 BIG-IP", Ordered, func() {
	ctx := context.Background()
	lbKey := types.NamespacedName{Name: drainLBName, Namespace: Namespace}
	member1 := drainNode1IP + ":80"
	member2 := drainNode2IP + ":80"
	var sim *f5Simulator

	// drainingHosts returns the hosts of the members being drained according to the instance status
	drainingHosts := func(g Gomega) []string {
		lb := &lbv1.ExternalLoadBalancer{}
		g.Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		hosts := make([]string, 0, len(lb.Status.DrainingMembers))
		for _, dm := range lb.Status.DrainingMembers {
			g.Expect(dm.PoolName).To(Equal(drainPoolName))
			hosts = append(hosts, dm.Node.Host)
		}
		return hosts
	}

	setDrainLabel := func(nodeName string, present bool) {
		Eventually(func() error {
			node := &corev1.Node{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
				return err
			}
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}
			if present {
				node.Labels[drainNodeLabel] = drainNodeLabelValue
			} else {
				delete(node.Labels, drainNodeLabel)
			}
			return k8sClient.Update(ctx, node)
		}, timeout, interval).Should(Succeed())
	}

	memberSession := func(member string) func() string {
		return func() string { return sim.memberSession(drainPoolName, member) }
	}

	BeforeAll(func() {
		sim = newF5Simulator()
		host, port, err := sim.hostPort()
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: drainCredsName, Namespace: Namespace},
			Data:       credsSecret.Data,
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(drainNode1Name, drainNodeLabels(), drainNode1IP))).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(drainNode2Name, drainNodeLabels(), drainNode2IP))).To(Succeed())
		Expect(k8sClient.Create(ctx, &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: drainLBName, Namespace: Namespace},
			Spec:       drainTestSpec(lbv1.Provider{Vendor: "F5_BigIP", Host: host, Port: port, Creds: drainCredsName}),
		})).To(Succeed())
	})

	AfterAll(func() {
		lb := &lbv1.ExternalLoadBalancer{}
		if err := k8sClient.Get(ctx, lbKey, lb); err == nil {
			Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
			}, timeout, interval).Should(BeTrue())
		}
		for _, name := range []string{drainNode1Name, drainNode2Name} {
			Expect(k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
		}
		Expect(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: drainCredsName, Namespace: Namespace}})).To(Succeed())
		sim.close()
	})

	It("Should configure the pool with both members enabled", func() {
		Eventually(memberSession(member1), timeout, interval).Should(Equal(simSessionMonitorEnabled))
		Eventually(memberSession(member2), timeout, interval).Should(Equal(simSessionMonitorEnabled))
		Eventually(drainingHosts, timeout, interval).Should(BeEmpty())
	})

	It("Should disable a removed member and delete it after the drain timeout", func() {
		setDrainLabel(drainNode2Name, false)

		By("disabling the member while it drains")
		Eventually(memberSession(member2), timeout, interval).Should(Equal(simSessionUserDisabled))
		Eventually(drainingHosts, timeout, interval).Should(ConsistOf(drainNode2IP))
		Expect(memberSession(member1)()).To(Equal(simSessionMonitorEnabled))

		By("deleting the member once the drain timeout expired")
		Eventually(func() bool {
			_, exists := sim.member(drainPoolName, member2)
			return exists
		}, timeout, interval).Should(BeFalse())
		Eventually(drainingHosts, timeout, interval).Should(BeEmpty())

		disabledAt, ok := sim.eventTime(simDisabled, member2)
		Expect(ok).To(BeTrue())
		deletedAt, ok := sim.eventTime(simDeleted, member2)
		Expect(ok).To(BeTrue())
		// Measured from the first disable, when the member stopped receiving new connections
		Expect(deletedAt.Sub(disabledAt)).To(BeNumerically(">=", drainTimeoutSeconds*time.Second))
		// A reconciliation reading the instance from a cache that does not have the previous status update
		// yet disables the member again, which is harmless since it only makes the drain last longer
		Expect(sim.count(simDisabled, member2)).To(BeNumerically(">=", 1))
	})

	It("Should re-enable a draining member when its node is added back", func() {
		By("using a drain timeout long enough to add the node back while it drains")
		Eventually(func() error {
			lb := &lbv1.ExternalLoadBalancer{}
			if err := k8sClient.Get(ctx, lbKey, lb); err != nil {
				return err
			}
			lb.Spec.Drain.TimeoutSeconds = 300
			return k8sClient.Update(ctx, lb)
		}, timeout, interval).Should(Succeed())

		setDrainLabel(drainNode1Name, false)
		Eventually(memberSession(member1), timeout, interval).Should(Equal(simSessionUserDisabled))
		Eventually(drainingHosts, timeout, interval).Should(ConsistOf(drainNode1IP))

		setDrainLabel(drainNode1Name, true)
		Eventually(memberSession(member1), timeout, interval).Should(Equal(simSessionUserEnabled))
		Eventually(drainingHosts, timeout, interval).Should(BeEmpty())
		Expect(sim.count(simEnabled, member1)).To(BeNumerically(">=", 1))
		Expect(sim.count(simDeleted, member1)).To(BeZero())
		Expect(sim.count(simCreated, member1)).To(Equal(1))
	})

	It("Should remove the load balancer configuration when the instance is deleted", func() {
		lb := &lbv1.ExternalLoadBalancer{}
		Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
		}, timeout, interval).Should(BeTrue())
		// A reconciliation still reading the deleted instance from its cache must not configure it again
		Consistently(func() bool { return sim.poolExists(drainPoolName) }, 2*time.Second, interval).Should(BeFalse())
	})
})
