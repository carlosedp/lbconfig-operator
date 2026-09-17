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

package controller_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lbv1 "github.com/carlosedp/lbconfig-operator/api/v1"
	. "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_controller"
	_ "github.com/carlosedp/lbconfig-operator/internal/controller/backend/backend_loader"
	d "github.com/carlosedp/lbconfig-operator/internal/controller/backend/dummy"
)

func TestBackendController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backend Controller Suite")
}

const (
	dummyHostIP = "1.2.3.4"
	testNodeIP  = "1.1.1.1"
	helperPool  = "test-pool"
)

var loadBalancer = &lbv1.ExternalLoadBalancer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "dummy-backend",
		Namespace: "default",
	},
	Spec: lbv1.ExternalLoadBalancerSpec{
		Vip: "10.0.0.1",
		Provider: lbv1.Provider{
			Vendor: "Dummy",
			Host:   dummyHostIP,
			Port:   443,
			Creds:  "secretname",
		},
	},
}

var monitor = lbv1.Monitor{
	Path:        "/",
	Port:        80,
	MonitorType: "http",
}

var pool = &lbv1.Pool{
	Name: "test-pool",
	Members: []lbv1.PoolMember{{
		Node: lbv1.Node{
			Name:   "test-node-1",
			Host:   testNodeIP,
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

var VIP = &lbv1.VIP{
	Name: "test-vip",
	Pool: pool.Name,
	IP:   dummyHostIP,
}

// ----------------------------------------
// Stateful mock provider
// ----------------------------------------

const (
	mockVendor    = "StatefulMock"
	drainPoolName = "Pool-drain-80"
	drainTimeout  = 30 * time.Second
)

// Pool member operations recorded by the stateful mock provider
const (
	opCreate  = "CreatePoolMember"
	opEdit    = "EditPoolMember"
	opDisable = "DisablePoolMember"
	opDelete  = "DeletePoolMember"
)

// mockCall is a pool member operation executed on the stateful mock provider
type mockCall struct {
	op     string
	member string
}

// mockMember is a pool member configured on the stateful mock provider
type mockMember struct {
	host    string
	port    int
	enabled bool
}

// statefulProvider is an in-memory load balancer keeping the pools and their members state.
// Unlike the Dummy provider it reports existing pools, so HandlePool goes through the pool update
// path where members are drained.
type statefulProvider struct {
	pools    map[string]map[string]*mockMember
	calls    []mockCall
	failures map[string]error
}

var mockProvider = &statefulProvider{}

func init() {
	if err := RegisterProvider(mockVendor, mockProvider); err != nil {
		panic(err)
	}
}

func memberKey(m *lbv1.PoolMember) string {
	return m.Node.Host + ":" + strconv.Itoa(m.Port)
}

func (p *statefulProvider) reset() {
	p.pools = map[string]map[string]*mockMember{}
	p.calls = nil
	p.failures = map[string]error{}
}

// configured reports if the member exists in the pool
func (p *statefulProvider) configured(pool string, m lbv1.PoolMember) bool {
	_, ok := p.pools[pool][memberKey(&m)]
	return ok
}

// enabled reports if the member exists in the drain test pool and accepts new connections
func (p *statefulProvider) enabled(m lbv1.PoolMember) bool {
	mm, ok := p.pools[drainPoolName][memberKey(&m)]
	return ok && mm.enabled
}

// removeMember deletes a member out of band, like an administrator would on the load balancer
func (p *statefulProvider) removeMember(pool string, m lbv1.PoolMember) {
	delete(p.pools[pool], memberKey(&m))
}

// count returns how many times the operation was executed for the member
func (p *statefulProvider) count(op string, m lbv1.PoolMember) int {
	n := 0
	for _, c := range p.calls {
		if c.op == op && c.member == memberKey(&m) {
			n++
		}
	}
	return n
}

func (p *statefulProvider) record(op string, m *lbv1.PoolMember) error {
	p.calls = append(p.calls, mockCall{op: op, member: memberKey(m)})
	return p.failures[op]
}

func (p *statefulProvider) setEnabled(m *lbv1.PoolMember, pool *lbv1.Pool, enabled bool) error {
	mm, ok := p.pools[pool.Name][memberKey(m)]
	if !ok {
		return fmt.Errorf("object not found - %s", memberKey(m))
	}
	mm.enabled = enabled
	return nil
}

func (p *statefulProvider) Create(context.Context, lbv1.Provider, string, string) error { return nil }

func (p *statefulProvider) Connect() error { return nil }

func (p *statefulProvider) Close() error { return nil }

func (p *statefulProvider) GetMonitor(m *lbv1.Monitor) (*lbv1.Monitor, error) { return m, nil }

func (p *statefulProvider) CreateMonitor(*lbv1.Monitor) error { return nil }

func (p *statefulProvider) EditMonitor(*lbv1.Monitor) error { return nil }

func (p *statefulProvider) DeleteMonitor(*lbv1.Monitor) error { return nil }

func (p *statefulProvider) GetPool(pool *lbv1.Pool) (*lbv1.Pool, error) {
	if _, ok := p.pools[pool.Name]; !ok {
		return nil, nil
	}
	return &lbv1.Pool{Name: pool.Name, Monitor: pool.Monitor}, nil
}

func (p *statefulProvider) CreatePool(pool *lbv1.Pool) error {
	p.pools[pool.Name] = map[string]*mockMember{}
	return nil
}

func (p *statefulProvider) EditPool(*lbv1.Pool) error { return nil }

func (p *statefulProvider) DeletePool(pool *lbv1.Pool) error {
	delete(p.pools, pool.Name)
	return nil
}

// GetPoolMembers reports members named after their address (like F5 does) instead of the Kubernetes node name
func (p *statefulProvider) GetPoolMembers(pool *lbv1.Pool) (*lbv1.Pool, error) {
	keys := make([]string, 0, len(p.pools[pool.Name]))
	for k := range p.pools[pool.Name] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	members := make([]lbv1.PoolMember, 0, len(keys))
	for _, k := range keys {
		mm := p.pools[pool.Name][k]
		members = append(members, lbv1.PoolMember{Node: lbv1.Node{Name: k, Host: mm.host}, Port: mm.port})
	}
	return &lbv1.Pool{Name: pool.Name, Monitor: pool.Monitor, Members: members}, nil
}

func (p *statefulProvider) CreatePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	if err := p.record(opCreate, m); err != nil {
		return err
	}
	p.pools[pool.Name][memberKey(m)] = &mockMember{host: m.Node.Host, port: m.Port, enabled: true}
	return nil
}

func (p *statefulProvider) EditPoolMember(m *lbv1.PoolMember, pool *lbv1.Pool, status string) error {
	if err := p.record(opEdit, m); err != nil {
		return err
	}
	return p.setEnabled(m, pool, status == "enable")
}

func (p *statefulProvider) DisablePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	if err := p.record(opDisable, m); err != nil {
		return err
	}
	return p.setEnabled(m, pool, false)
}

func (p *statefulProvider) DeletePoolMember(m *lbv1.PoolMember, pool *lbv1.Pool) error {
	if err := p.record(opDelete, m); err != nil {
		return err
	}
	if !p.configured(pool.Name, *m) {
		return fmt.Errorf("object not found - %s", memberKey(m))
	}
	p.removeMember(pool.Name, *m)
	return nil
}

func (p *statefulProvider) GetVIP(*lbv1.VIP) (*lbv1.VIP, error) { return nil, nil }

func (p *statefulProvider) CreateVIP(*lbv1.VIP) error { return nil }

func (p *statefulProvider) EditVIP(*lbv1.VIP) error { return nil }

func (p *statefulProvider) DeleteVIP(*lbv1.VIP) error { return nil }

// drainTestMember builds a pool member for a Kubernetes node
func drainTestMember(nodeName, host string) lbv1.PoolMember {
	return lbv1.PoolMember{Node: lbv1.Node{Name: nodeName, Host: host}, Port: 80}
}

// desiredPool builds the pool the controller wants configured with the given members
func desiredPool(members ...lbv1.PoolMember) *lbv1.Pool {
	return &lbv1.Pool{Name: drainPoolName, Monitor: "Monitor-drain", Members: members}
}

// drainingSince builds a draining member entry as persisted in the ExternalLoadBalancer status
func drainingSince(m lbv1.PoolMember, elapsed time.Duration) lbv1.DrainingMember {
	return lbv1.DrainingMember{
		PoolName:  drainPoolName,
		Node:      m.Node,
		Port:      m.Port,
		StartTime: metav1.NewTime(time.Now().Add(-elapsed)),
	}
}

var _ = Describe("Controllers/Backend/controller/backend_controller", func() {

	Context("When using a creating backends", func() {
		var ctx = context.TODO()

		It("Should validate that backend provider is registered", func() {
			Expect(ListProviders()).Should(ContainElement("dummy"))
		})

		It("Should return error if backend provider tries to register again", func() {
			err := RegisterProvider("Dummy", new(d.DummyProvider))
			Expect(err).To(MatchError(MatchRegexp("provider already exists.*")))
		})

		It("Should return error if backend provider does not exist", func() {
			loadBalancer := &lbv1.ExternalLoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-backend",
					Namespace: "default",
				},
				Spec: lbv1.ExternalLoadBalancerSpec{
					Vip: "10.0.0.1",
					Provider: lbv1.Provider{
						Vendor: "unknown",
						Host:   dummyHostIP,
						Port:   443,
						Creds:  "secretname",
					},
				},
			}
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError(MatchRegexp("no such provider.*")))
			Expect(createdBackend).To(BeNil())
		})

		It("Should create a provider with registered backend provider", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reflect.TypeOf(createdBackend.Provider)).Should(Equal(reflect.TypeOf(&d.DummyProvider{})))
		})

		It("Should handle a provider monitor", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).ShouldNot(HaveOccurred())
			err = createdBackend.HandleMonitors(ctx, &monitor)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should handle a provider pool", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).ShouldNot(HaveOccurred())
			requeueAfter, drainingMembers, err := createdBackend.HandlePool(ctx, pool, &monitor, loadBalancer, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(requeueAfter).Should(BeZero())
			Expect(drainingMembers).Should(BeEmpty())
		})

		It("Should handle a provider VIP", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).ShouldNot(HaveOccurred())
			err = createdBackend.HandleVIP(ctx, VIP)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should handle a provider cleanup", func() {
			createdBackend, err := CreateBackend(ctx, &loadBalancer.Spec.Provider, "username", "password")
			Expect(err).ShouldNot(HaveOccurred())
			err = createdBackend.HandleCleanup(ctx, loadBalancer)
			Expect(err).ShouldNot(HaveOccurred())
		})

	})
	Context("When using auxiliary functions", func() {
		It("Should return true if array contains member", func() {
			m := lbv1.PoolMember{
				Node: lbv1.Node{
					Name: "node1",
					Host: testNodeIP,
				},
				Port: 80,
			}
			a := []lbv1.PoolMember{m}
			output := ContainsMember(a, m)
			Expect(output).To(BeTrue())
		})

		It("Should return false if array doesn't contain member", func() {
			m := lbv1.PoolMember{
				Node: lbv1.Node{
					Name: "node1",
					Host: testNodeIP,
				},
				Port: 80,
			}
			m2 := m.DeepCopy()
			m2.Node.Host = "1.1.1.2"
			a := []lbv1.PoolMember{m}
			output := ContainsMember(a, *m2)
			Expect(output).To(BeFalse())
		})
	})

	Context("When using drain helper functions", func() {
		var testMember lbv1.PoolMember
		var testDrainingMember lbv1.DrainingMember
		var drainingList []lbv1.DrainingMember

		BeforeEach(func() {
			testMember = lbv1.PoolMember{
				Node: lbv1.Node{
					Name: "test-node",
					Host: "10.0.1.100",
				},
				Port: 6443,
			}

			testDrainingMember = lbv1.DrainingMember{
				PoolName:  helperPool,
				Node:      testMember.Node,
				Port:      testMember.Port,
				StartTime: metav1.Now(),
			}

			drainingList = []lbv1.DrainingMember{testDrainingMember}
		})

		Describe("IsMemberDraining", func() {
			It("Should return nil when member is not in draining list", func() {
				emptyList := []lbv1.DrainingMember{}
				result := IsMemberDraining(emptyList, &testMember, helperPool)
				Expect(result).To(BeNil())
			})

			It("Should return nil when member host does not match", func() {
				differentMember := testMember
				differentMember.Node.Host = "10.0.1.200"
				result := IsMemberDraining(drainingList, &differentMember, helperPool)
				Expect(result).To(BeNil())
			})

			It("Should return nil when member port does not match", func() {
				differentMember := testMember
				differentMember.Port = 8443
				result := IsMemberDraining(drainingList, &differentMember, helperPool)
				Expect(result).To(BeNil())
			})

			It("Should return nil when pool name does not match", func() {
				result := IsMemberDraining(drainingList, &testMember, "different-pool")
				Expect(result).To(BeNil())
			})

			It("Should return draining member when all criteria match", func() {
				result := IsMemberDraining(drainingList, &testMember, helperPool)
				Expect(result).NotTo(BeNil())
				Expect(result.PoolName).To(Equal(helperPool))
				Expect(result.Node.Host).To(Equal("10.0.1.100"))
				Expect(result.Port).To(Equal(6443))
			})

			It("Should find correct member in list with multiple draining members", func() {
				member2 := lbv1.PoolMember{
					Node: lbv1.Node{Name: "node2", Host: "10.0.1.101"},
					Port: 6443,
				}
				draining2 := lbv1.DrainingMember{
					PoolName:  helperPool,
					Node:      member2.Node,
					Port:      member2.Port,
					StartTime: metav1.Now(),
				}
				multiList := append(drainingList, draining2)

				result := IsMemberDraining(multiList, &member2, helperPool)
				Expect(result).NotTo(BeNil())
				Expect(result.Node.Host).To(Equal("10.0.1.101"))
			})
		})

		Describe("RemoveDrainingMember", func() {
			It("Should return empty list when removing only member", func() {
				result := RemoveDrainingMember(drainingList, &testMember, helperPool)
				Expect(result).To(BeEmpty())
			})

			It("Should return unchanged list when member not found", func() {
				differentMember := testMember
				differentMember.Node.Host = "10.0.1.200"
				result := RemoveDrainingMember(drainingList, &differentMember, helperPool)
				Expect(result).To(HaveLen(1))
				Expect(result[0].Node.Host).To(Equal("10.0.1.100"))
			})

			It("Should remove only matching member from list", func() {
				member2 := lbv1.PoolMember{
					Node: lbv1.Node{Name: "node2", Host: "10.0.1.101"},
					Port: 6443,
				}
				draining2 := lbv1.DrainingMember{
					PoolName:  helperPool,
					Node:      member2.Node,
					Port:      member2.Port,
					StartTime: metav1.Now(),
				}
				multiList := append(drainingList, draining2)

				result := RemoveDrainingMember(multiList, &testMember, helperPool)
				Expect(result).To(HaveLen(1))
				Expect(result[0].Node.Host).To(Equal("10.0.1.101"))
			})

			It("Should preserve members from different pools", func() {
				differentPoolMember := lbv1.DrainingMember{
					PoolName:  "different-pool",
					Node:      testMember.Node,
					Port:      testMember.Port,
					StartTime: metav1.Now(),
				}
				multiList := append(drainingList, differentPoolMember)

				result := RemoveDrainingMember(multiList, &testMember, helperPool)
				Expect(result).To(HaveLen(1))
				Expect(result[0].PoolName).To(Equal("different-pool"))
			})
		})
	})

	Context("When removing members from an existing pool", func() {
		var ctx context.Context
		var backend *BackendController
		var lb *lbv1.ExternalLoadBalancer
		node1 := drainTestMember("node-1", "10.0.1.1")
		node2 := drainTestMember("node-2", "10.0.1.2")

		BeforeEach(func() {
			ctx = context.TODO()
			mockProvider.reset()

			var err error
			backend, err = CreateBackend(ctx, &lbv1.Provider{Vendor: mockVendor, Host: dummyHostIP, Port: 443, Creds: loadBalancer.Spec.Provider.Creds}, "username", "password")
			Expect(err).ToNot(HaveOccurred())
			lb = &lbv1.ExternalLoadBalancer{
				Spec: lbv1.ExternalLoadBalancerSpec{
					Drain: &lbv1.DrainConfig{Enabled: true, TimeoutSeconds: int(drainTimeout.Seconds())},
				},
			}

			By("creating the pool with both members enabled")
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1, node2), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.enabled(node1)).To(BeTrue())
			Expect(mockProvider.enabled(node2)).To(BeTrue())
		})

		DescribeTable("Should delete removed members right away when draining is off",
			func(drain *lbv1.DrainConfig) {
				lb.Spec.Drain = drain
				requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(requeueAfter).To(BeZero())
				Expect(drainingMembers).To(BeEmpty())
				Expect(mockProvider.configured(drainPoolName, node2)).To(BeFalse())
				Expect(mockProvider.count(opDisable, node2)).To(BeZero())
				Expect(mockProvider.count(opDelete, node2)).To(Equal(1))
			},
			Entry("drain not configured", nil),
			Entry("drain disabled", &lbv1.DrainConfig{Enabled: false, TimeoutSeconds: 30}),
		)

		It("Should disable a removed member and delete it only after the drain timeout", func() {
			By("disabling the member when its node is removed")
			before := time.Now()
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(And(BeNumerically(">", drainTimeout), BeNumerically("<=", drainTimeout+time.Second)))
			Expect(drainingMembers).To(HaveLen(1))
			// Rounded up to the second kept by the status, so the drain is never shortened
			Expect(drainingMembers[0].StartTime.Time).To(BeTemporally(">", before))
			Expect(drainingMembers[0].StartTime.Nanosecond()).To(BeZero())
			Expect(drainingMembers[0].PoolName).To(Equal(drainPoolName))
			Expect(drainingMembers[0].Node.Host).To(Equal(node2.Node.Host))
			Expect(drainingMembers[0].Port).To(Equal(node2.Port))
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeTrue())
			Expect(mockProvider.enabled(node2)).To(BeFalse())
			Expect(mockProvider.enabled(node1)).To(BeTrue())

			By("keeping the disabled member while the drain timeout has not expired")
			startTime := drainingMembers[0].StartTime
			requeueAfter, drainingMembers, err = backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(And(BeNumerically(">", 0), BeNumerically("<=", drainTimeout+time.Second)))
			Expect(drainingMembers).To(HaveLen(1))
			Expect(drainingMembers[0].StartTime).To(Equal(startTime))
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeTrue())
			Expect(mockProvider.count(opDisable, node2)).To(Equal(1))
			Expect(mockProvider.count(opDelete, node2)).To(BeZero())

			By("deleting the member once the drain timeout expired")
			drainingMembers[0].StartTime = metav1.NewTime(time.Now().Add(-drainTimeout - time.Second))
			requeueAfter, drainingMembers, err = backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeFalse())
			Expect(mockProvider.count(opDelete, node2)).To(Equal(1))
			Expect(mockProvider.enabled(node1)).To(BeTrue())
		})

		It("Should resume a drain from the start time persisted in the status", func() {
			// Drain state restored from the status after an operator restart, 20s into a 30s drain
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb,
				[]lbv1.DrainingMember{drainingSince(node2, 20*time.Second)})
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeNumerically("~", 10*time.Second, time.Second))
			Expect(drainingMembers).To(HaveLen(1))
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeTrue())
			Expect(mockProvider.count(opDisable, node2)).To(BeZero())
		})

		It("Should re-enable a draining member when its node is added back", func() {
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(mockProvider.enabled(node2)).To(BeFalse())

			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1, node2), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.enabled(node2)).To(BeTrue())
			Expect(mockProvider.count(opEdit, node2)).To(Equal(1))
			Expect(mockProvider.count(opDelete, node2)).To(BeZero())
			Expect(mockProvider.count(opCreate, node2)).To(Equal(1), "only created when the pool was created")
		})

		It("Should recreate a draining member deleted from the load balancer when its node is added back", func() {
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			mockProvider.removeMember(drainPoolName, node2)

			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1, node2), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.enabled(node2)).To(BeTrue())
			Expect(mockProvider.count(opEdit, node2)).To(BeZero())
			Expect(mockProvider.count(opCreate, node2)).To(Equal(2))
		})

		It("Should stop tracking a draining member deleted from the load balancer", func() {
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			mockProvider.removeMember(drainPoolName, node2)

			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.count(opDelete, node2)).To(BeZero())
		})

		It("Should delete draining members right away when draining gets disabled", func() {
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())

			lb.Spec.Drain.Enabled = false
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, drainingMembers)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeFalse())
		})

		It("Should requeue for the draining member closest to its drain timeout", func() {
			// node-2 is 25s into its drain when node-1 is also removed
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(), &monitor, lb,
				[]lbv1.DrainingMember{drainingSince(node2, 25*time.Second)})
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeNumerically("~", 5*time.Second, time.Second))
			Expect(drainingMembers).To(HaveLen(2))
			Expect(mockProvider.enabled(node1)).To(BeFalse())
			Expect(mockProvider.configured(drainPoolName, node2)).To(BeTrue())
		})

		It("Should use the default drain timeout when none is set", func() {
			lb.Spec.Drain = &lbv1.DrainConfig{Enabled: true}
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeNumerically("~", DefaultDrainTimeoutSeconds*time.Second, time.Second))
			Expect(drainingMembers).To(HaveLen(1))
		})

		It("Should keep the draining members of other pools", func() {
			otherPool := drainingSince(node2, time.Minute)
			otherPool.PoolName = "Pool-drain-443"
			requeueAfter, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1, node2), &monitor, lb, []lbv1.DrainingMember{otherPool})
			Expect(err).ToNot(HaveOccurred())
			Expect(requeueAfter).To(BeZero())
			Expect(drainingMembers).To(ConsistOf(otherPool))
			Expect(mockProvider.count(opEdit, node2)).To(BeZero())
		})

		It("Should not track a member it failed to disable", func() {
			mockProvider.failures[opDisable] = fmt.Errorf("disable failed")
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).To(MatchError(ContainSubstring("disable failed")))
			Expect(drainingMembers).To(BeEmpty())
			Expect(mockProvider.enabled(node2)).To(BeTrue())
		})

		It("Should keep tracking a re-added member it failed to re-enable", func() {
			_, drainingMembers, err := backend.HandlePool(ctx, desiredPool(node1), &monitor, lb, nil)
			Expect(err).ToNot(HaveOccurred())

			mockProvider.failures[opEdit] = fmt.Errorf("enable failed")
			_, drainingMembers, err = backend.HandlePool(ctx, desiredPool(node1, node2), &monitor, lb, drainingMembers)
			Expect(err).To(MatchError(ContainSubstring("enable failed")))
			Expect(drainingMembers).To(HaveLen(1))
			Expect(mockProvider.enabled(node2)).To(BeFalse())
		})
	})
})
