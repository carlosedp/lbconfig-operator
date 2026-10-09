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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

const (
	optionalLBName         = "optional-f5"
	optionalInvalidLBName  = "optional-f5-invalid"
	optionalCredsName      = "optional-f5-creds"
	optionalPoolName       = "Pool-" + optionalLBName + "-80"
	optionalVIPName        = "VIP-" + optionalLBName + "-80"
	optionalMonitorName    = "Monitor-" + optionalLBName
	optionalSharedMonitor  = "shared-monitor"
	optionalNodeLabel      = "lbconfig.carlosedp.com/optional-test"
	optionalNodeLabelValue = "true"
	optionalNodeName       = "optional-node-1"
	optionalNodeIP         = "10.20.0.1"
)

// optionalTestSpec builds a valid ExternalLoadBalancer spec for the given provider, without draining
func optionalTestSpec(provider lbv1.Provider) lbv1.ExternalLoadBalancerSpec {
	spec := drainTestSpec(provider)
	spec.Vip = "10.20.0.100"
	spec.NodeLabels = map[string]string{optionalNodeLabel: optionalNodeLabelValue}
	spec.Drain = nil
	return spec
}

var _ = Describe("ExternalLoadBalancer optional VIP and monitor configuration", func() {
	ctx := context.Background()
	provider := loadBalancer.Spec.Provider

	It("Should default to creating the VIP and the monitor", func() {
		lb := &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: "optional-defaults", Namespace: Namespace},
			Spec:       optionalTestSpec(provider),
		}
		Expect(k8sClient.Create(ctx, lb, client.DryRunAll)).To(Succeed())
		Expect(lb.Spec.CreateVIP).To(HaveValue(BeTrue()))
		Expect(lb.Spec.CreateMonitor).To(HaveValue(BeTrue()))
	})

	It("Should keep the VIP and monitor creation turned off", func() {
		lb := &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: "optional-disabled", Namespace: Namespace},
			Spec:       optionalTestSpec(provider),
		}
		lb.Spec.CreateVIP = ptr.To(false)
		lb.Spec.CreateMonitor = ptr.To(false)
		Expect(k8sClient.Create(ctx, lb, client.DryRunAll)).To(Succeed())
		Expect(lb.Spec.ShouldCreateVIP()).To(BeFalse())
		Expect(lb.Spec.ShouldCreateMonitor()).To(BeFalse())
	})
})

var _ = Describe("ExternalLoadBalancer optional VIP and monitor with a F5 BIG-IP", Ordered, func() {
	ctx := context.Background()
	lbKey := types.NamespacedName{Name: optionalLBName, Namespace: Namespace}
	var sim *f5Simulator

	// updateSpec changes the instance spec, retrying on conflicts with the controller status updates
	updateSpec := func(change func(spec *lbv1.ExternalLoadBalancerSpec)) {
		Eventually(func() error {
			lb := &lbv1.ExternalLoadBalancer{}
			if err := k8sClient.Get(ctx, lbKey, lb); err != nil {
				return err
			}
			change(&lb.Spec)
			return k8sClient.Update(ctx, lb)
		}, timeout, interval).Should(Succeed())
	}

	getStatus := func(g Gomega) lbv1.ExternalLoadBalancerStatus {
		lb := &lbv1.ExternalLoadBalancer{}
		g.Expect(k8sClient.Get(ctx, lbKey, lb)).To(Succeed())
		return lb.Status
	}

	deleteInstance := func(key types.NamespacedName) {
		lb := &lbv1.ExternalLoadBalancer{}
		if err := k8sClient.Get(ctx, key, lb); err != nil {
			return
		}
		Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, key, &lbv1.ExternalLoadBalancer{}))
		}, timeout, interval).Should(BeTrue())
	}

	var provider lbv1.Provider

	BeforeAll(func() {
		sim = newF5Simulator()
		host, port, err := sim.hostPort()
		Expect(err).NotTo(HaveOccurred())
		provider = lbv1.Provider{Vendor: "F5_BigIP", Host: host, Port: port, Creds: optionalCredsName}

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: optionalCredsName, Namespace: Namespace},
			Data:       credsSecret.Data,
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(optionalNodeName,
			map[string]string{optionalNodeLabel: optionalNodeLabelValue}, optionalNodeIP))).To(Succeed())
		Expect(k8sClient.Create(ctx, &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: optionalLBName, Namespace: Namespace},
			Spec:       optionalTestSpec(provider),
		})).To(Succeed())
	})

	AfterAll(func() {
		deleteInstance(lbKey)
		deleteInstance(types.NamespacedName{Name: optionalInvalidLBName, Namespace: Namespace})
		Expect(k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: optionalNodeName}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: optionalCredsName, Namespace: Namespace}})).To(Succeed())
		sim.close()
	})

	It("Should create the VIP and the monitor by default", func() {
		Eventually(func() bool { return sim.virtualExists(optionalVIPName) }, timeout, interval).Should(BeTrue())
		Expect(sim.monitorExists(optionalMonitorName)).To(BeTrue())
		Eventually(func() string { return sim.poolMonitor(optionalPoolName) }, timeout, interval).
			Should(Equal(simPartitionPath + optionalMonitorName))
	})

	It("Should remove the VIP and keep the pool when VIP creation is turned off", func() {
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) { spec.CreateVIP = ptr.To(false) })
		Eventually(func() bool { return sim.virtualExists(optionalVIPName) }, timeout, interval).Should(BeFalse())
		Eventually(func(g Gomega) { g.Expect(getStatus(g).VIPs).To(BeEmpty()) }, timeout, interval).Should(Succeed())
		Expect(sim.poolExists(optionalPoolName)).To(BeTrue())
		Expect(sim.memberSession(optionalPoolName, optionalNodeIP+":80")).NotTo(BeEmpty())
	})

	It("Should use an existing monitor and remove its own when monitor creation is turned off", func() {
		sim.addMonitor(optionalSharedMonitor)
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) {
			spec.CreateMonitor = ptr.To(false)
			spec.Monitor.Name = optionalSharedMonitor
		})
		Eventually(func() string { return sim.poolMonitor(optionalPoolName) }, timeout, interval).
			Should(Equal(simPartitionPath + optionalSharedMonitor))
		Eventually(func() bool { return sim.monitorExists(optionalMonitorName) }, timeout, interval).Should(BeFalse())
		Eventually(func(g Gomega) { g.Expect(getStatus(g).Monitor).To(BeNil()) }, timeout, interval).Should(Succeed())
		Expect(sim.virtualExists(optionalVIPName)).To(BeFalse())
	})

	It("Should not update the pool on every reconcile when the existing monitor is named with its partition", func() {
		updates := sim.count(simPoolUpdated, optionalPoolName)
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) {
			spec.Monitor.Name = simPartitionPath + optionalSharedMonitor
		})
		// A spec change the pool does not depend on, to run another reconcile
		updateSpec(func(spec *lbv1.ExternalLoadBalancerSpec) { spec.Monitor.Path = "/ready" })
		Consistently(func() int { return sim.count(simPoolUpdated, optionalPoolName) }, 2*time.Second, interval).
			Should(Equal(updates))
		Expect(sim.poolMonitor(optionalPoolName)).To(Equal(simPartitionPath + optionalSharedMonitor))
	})

	It("Should keep the existing monitor when the instance is deleted", func() {
		deleteInstance(lbKey)
		Eventually(func() bool { return sim.poolExists(optionalPoolName) }, timeout, interval).Should(BeFalse())
		Expect(sim.monitorExists(optionalSharedMonitor)).To(BeTrue())
	})

	It("Should not configure the load balancer when monitor creation is off and no monitor is named", func() {
		spec := optionalTestSpec(provider)
		spec.CreateMonitor = ptr.To(false)
		Expect(k8sClient.Create(ctx, &lbv1.ExternalLoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: optionalInvalidLBName, Namespace: Namespace},
			Spec:       spec,
		})).To(Succeed())
		Consistently(func() bool { return sim.poolExists("Pool-" + optionalInvalidLBName + "-80") }, 2*time.Second, interval).
			Should(BeFalse())
	})
})
