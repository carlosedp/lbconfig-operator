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

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
)

const (
	firstReconcileLBName    = "first-reconcile-f5"
	firstReconcileCredsName = "first-reconcile-f5-creds"
	firstReconcileNodeName  = "first-reconcile-node-1"
	firstReconcileNodeLabel = "lbconfig.carlosedp.com/first-reconcile-test"
)

var _ = Describe("ExternalLoadBalancer deleted during its first reconciliation", Ordered, func() {
	ctx := context.Background()
	lbKey := types.NamespacedName{Name: firstReconcileLBName, Namespace: Namespace}
	poolName := "Pool-" + firstReconcileLBName + "-80"
	var sim *f5Simulator

	BeforeAll(func() {
		sim = newF5Simulator()
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: firstReconcileCredsName, Namespace: Namespace},
			Data:       credsSecret.Data,
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, createReadyNode(firstReconcileNodeName,
			map[string]string{firstReconcileNodeLabel: "true"}, "10.40.0.1"))).To(Succeed())
	})

	AfterAll(func() {
		Expect(k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: firstReconcileNodeName}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: firstReconcileCredsName, Namespace: Namespace}})).To(Succeed())
		sim.close()
	})

	It("Should remove the configuration it was applying when the instance was deleted", func() {
		release := sim.holdPoolCreation()
		host, port, err := sim.hostPort()
		Expect(err).NotTo(HaveOccurred())
		spec := drainTestSpec(lbv1.Provider{Vendor: simVendor, Host: host, Port: port, Creds: firstReconcileCredsName})
		spec.Vip = "10.40.0.100"
		spec.NodeLabels = map[string]string{firstReconcileNodeLabel: "true"}
		spec.Drain = nil
		lb := &lbv1.ExternalLoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: firstReconcileLBName, Namespace: Namespace}, Spec: spec}
		Expect(k8sClient.Create(ctx, lb)).To(Succeed())

		By("deleting the instance while the first reconciliation creates the pool")
		Eventually(sim.poolCreateStarted.Load, timeout, interval).Should(BeTrue())
		Expect(k8sClient.Delete(ctx, lb)).To(Succeed())
		release()

		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, lbKey, &lbv1.ExternalLoadBalancer{}))
		}, timeout, interval).Should(BeTrue())
		Eventually(func() bool { return sim.poolExists(poolName) }, timeout, interval).Should(BeFalse())
		Consistently(func() bool { return sim.poolExists(poolName) }, 2*time.Second, interval).Should(BeFalse())
	})
})
