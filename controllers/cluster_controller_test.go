/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/patch"
)

var _ = Describe("Cluster Reconciler", func() {

	It("Should create a Cluster", func() {
		instance := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test1-",
				Namespace:    "default",
			},
			Spec: clusterv1.ClusterSpec{},
		}

		// Create the Cluster object and expect the Reconcile and Deployment to be created
		Expect(testEnv.Create(ctx, instance)).ToNot(HaveOccurred())
		key := client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}
		defer func() {
			err := testEnv.Delete(ctx, instance)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Make sure the Cluster exists.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, instance); err != nil {
				return false
			}
			return len(instance.Finalizers) > 0
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if the status diff is empty but the spec diff is not", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test2-",
				Namespace:    "default",
			},
		}
		Expect(testEnv.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, testEnv)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Spec.InfrastructureRef = &v1.ObjectReference{Name: "test"}
			cluster.Spec.ControlPlaneRef = &v1.ObjectReference{Name: "test-too"}
			Expect(ph.Patch(ctx, cluster, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := testEnv.Get(ctx, key, instance); err != nil {
				return false
			}
			return instance.Spec.InfrastructureRef != nil &&
				instance.Spec.InfrastructureRef.Name == "test"
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if the spec diff is empty but the status diff is not", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test3-",
				Namespace:    "default",
			},
		}
		Expect(testEnv.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, testEnv)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Status.InfrastructureReady = true
			Expect(ph.Patch(ctx, cluster, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := testEnv.Get(ctx, key, instance); err != nil {
				return false
			}
			return instance.Status.InfrastructureReady
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if both the spec diff and status diff are non empty", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test4-",
				Namespace:    "default",
			},
		}
		Expect(testEnv.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, testEnv)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Status.InfrastructureReady = true
			cluster.Spec.InfrastructureRef = &v1.ObjectReference{Name: "test"}
			Expect(ph.Patch(ctx, cluster, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := testEnv.Get(ctx, key, instance); err != nil {
				return false
			}
			return instance.Status.InfrastructureReady &&
				instance.Spec.InfrastructureRef != nil &&
				instance.Spec.InfrastructureRef.Name == "test"
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if only removing finalizers", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test5-",
				Namespace:    "default",
			},
		}
		Expect(testEnv.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, testEnv)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.SetFinalizers([]string{})
			Expect(ph.Patch(ctx, cluster, patch.WithStatusObservedGeneration{})).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		Expect(cluster.Finalizers).Should(BeEmpty())

		// Assertions
		Eventually(func() []string {
			instance := &clusterv1.Cluster{}
			if err := testEnv.Get(ctx, key, instance); err != nil {
				return []string{"not-empty"}
			}
			return instance.Finalizers
		}, timeout).Should(BeEmpty())
	})

	It("Should successfully set Status.ControlPlaneInitialized on the cluster object if controlplane is ready", func() {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test6-",
				Namespace:    v1.NamespaceDefault,
			},
		}

		Expect(testEnv.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		}()
		Expect(testEnv.CreateKubeconfigSecret(cluster)).To(Succeed())

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Create a node so we can speed up reconciliation. Otherwise, the machine reconciler will requeue the machine
		// after 10 seconds, potentially slowing down this test.
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "id-node-1",
			},
			Spec: v1.NodeSpec{
				ProviderID: "aws:///id-node-1",
			},
		}

		Expect(testEnv.Create(ctx, node)).To(Succeed())

		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test6-",
				Namespace:    v1.NamespaceDefault,
				Labels: map[string]string{
					clusterv1.MachineControlPlaneLabelName: "",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				ProviderID:  pointer.StringPtr("aws:///id-node-1"),
				Bootstrap: clusterv1.Bootstrap{
					Data: pointer.StringPtr(""),
				},
			},
		}
		machine.Spec.Bootstrap.DataSecretName = pointer.StringPtr("test6-bootstrapdata")
		Expect(testEnv.Create(ctx, machine)).To(BeNil())
		key = client.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}
		defer func() {
			err := testEnv.Delete(ctx, machine)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for machine to be ready.
		//
		// [ncdc] Note, we're using an increased timeout because we've been seeing failures
		// in Prow for this particular block. It looks like it's sometimes taking more than 10 seconds (the value of
		// timeout) for the machine reconciler to add the finalizer and for the change to be persisted to etcd. If
		// we continue to see test timeouts here, that will likely point to something else being the problem, but
		// I've yet to determine any other possibility for the test flakes.
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, machine); err != nil {
				return false
			}
			return len(machine.Finalizers) > 0
		}, timeout*3).Should(BeTrue())

		// Assertion
		key = client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		Eventually(func() bool {
			if err := testEnv.Get(ctx, key, cluster); err != nil {
				return false
			}
			return cluster.Status.ControlPlaneInitialized
		}, timeout).Should(BeTrue())
	})
})

func TestClusterReconciler(t *testing.T) {
	t.Run("machine to cluster", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				Kind: "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test",
			},
			Spec:   clusterv1.ClusterSpec{},
			Status: clusterv1.ClusterStatus{},
		}

		controlPlaneWithNoderef := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "controlPlaneWithNoderef",
				Namespace: "test",
				Labels: map[string]string{
					clusterv1.ClusterLabelName:             cluster.Name,
					clusterv1.MachineControlPlaneLabelName: "",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &v1.ObjectReference{
					Kind:      "Node",
					Namespace: "test-node",
				},
			},
		}
		controlPlaneWithoutNoderef := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "controlPlaneWithoutNoderef",
				Namespace: "test",
				Labels: map[string]string{
					clusterv1.ClusterLabelName:             cluster.Name,
					clusterv1.MachineControlPlaneLabelName: "",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
			},
		}
		nonControlPlaneWithNoderef := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nonControlPlaneWitNoderef",
				Namespace: "test",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &v1.ObjectReference{
					Kind:      "Node",
					Namespace: "test-node",
				},
			},
		}
		nonControlPlaneWithoutNoderef := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nonControlPlaneWithoutNoderef",
				Namespace: "test",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
			},
		}

		tests := []struct {
			name string
			o    handler.MapObject
			want []ctrl.Request
		}{
			{
				name: "controlplane machine, noderef is set, should return cluster",
				o: handler.MapObject{
					Meta:   controlPlaneWithNoderef.GetObjectMeta(),
					Object: controlPlaneWithNoderef,
				},
				want: []ctrl.Request{
					{
						NamespacedName: util.ObjectKey(cluster),
					},
				},
			},
			{
				name: "controlplane machine, noderef is not set",
				o: handler.MapObject{
					Meta:   controlPlaneWithoutNoderef.GetObjectMeta(),
					Object: controlPlaneWithoutNoderef,
				},
				want: nil,
			},
			{
				name: "not controlplane machine, noderef is set",
				o: handler.MapObject{
					Meta:   nonControlPlaneWithNoderef.GetObjectMeta(),
					Object: nonControlPlaneWithNoderef,
				},
				want: nil,
			},
			{
				name: "not controlplane machine, noderef is not set",
				o: handler.MapObject{
					Meta:   nonControlPlaneWithoutNoderef.GetObjectMeta(),
					Object: nonControlPlaneWithoutNoderef,
				},
				want: nil,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

				r := &ClusterReconciler{
					Client: fake.NewFakeClientWithScheme(scheme.Scheme, cluster, controlPlaneWithNoderef, controlPlaneWithoutNoderef, nonControlPlaneWithNoderef, nonControlPlaneWithoutNoderef),
					Log:    log.Log,
				}
				requests := r.controlPlaneMachineToCluster(tt.o)
				g.Expect(requests).To(Equal(tt.want))
			})
		}
	})
}

type machineDeploymentBuilder struct {
	md clusterv1.MachineDeployment
}

func newMachineDeploymentBuilder() *machineDeploymentBuilder {
	return &machineDeploymentBuilder{}
}

func (b *machineDeploymentBuilder) named(name string) *machineDeploymentBuilder {
	b.md.Name = name
	return b
}

func (b *machineDeploymentBuilder) ownedBy(c *clusterv1.Cluster) *machineDeploymentBuilder {
	b.md.OwnerReferences = append(b.md.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       c.Name,
	})
	return b
}

func (b *machineDeploymentBuilder) build() clusterv1.MachineDeployment {
	return b.md
}

type machineSetBuilder struct {
	ms clusterv1.MachineSet
}

func newMachineSetBuilder() *machineSetBuilder {
	return &machineSetBuilder{}
}

func (b *machineSetBuilder) named(name string) *machineSetBuilder {
	b.ms.Name = name
	return b
}

func (b *machineSetBuilder) ownedBy(c *clusterv1.Cluster) *machineSetBuilder {
	b.ms.OwnerReferences = append(b.ms.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       c.Name,
	})
	return b
}

func (b *machineSetBuilder) build() clusterv1.MachineSet {
	return b.ms
}

type machineBuilder struct {
	m clusterv1.Machine
}

func newMachineBuilder() *machineBuilder {
	return &machineBuilder{}
}

func (b *machineBuilder) named(name string) *machineBuilder {
	b.m.Name = name
	return b
}

func (b *machineBuilder) ownedBy(c *clusterv1.Cluster) *machineBuilder {
	b.m.OwnerReferences = append(b.m.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       c.Name,
	})
	return b
}

func (b *machineBuilder) controlPlane() *machineBuilder {
	b.m.Labels = map[string]string{clusterv1.MachineControlPlaneLabelName: ""}
	return b
}

func (b *machineBuilder) build() clusterv1.Machine {
	return b.m
}

func TestFilterOwnedDescendants(t *testing.T) {
	g := NewWithT(t)

	c := clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "c",
		},
	}

	md1NotOwnedByCluster := newMachineDeploymentBuilder().named("md1").build()
	md2OwnedByCluster := newMachineDeploymentBuilder().named("md2").ownedBy(&c).build()
	md3NotOwnedByCluster := newMachineDeploymentBuilder().named("md3").build()
	md4OwnedByCluster := newMachineDeploymentBuilder().named("md4").ownedBy(&c).build()

	ms1NotOwnedByCluster := newMachineSetBuilder().named("ms1").build()
	ms2OwnedByCluster := newMachineSetBuilder().named("ms2").ownedBy(&c).build()
	ms3NotOwnedByCluster := newMachineSetBuilder().named("ms3").build()
	ms4OwnedByCluster := newMachineSetBuilder().named("ms4").ownedBy(&c).build()

	m1NotOwnedByCluster := newMachineBuilder().named("m1").build()
	m2OwnedByCluster := newMachineBuilder().named("m2").ownedBy(&c).build()
	m3ControlPlaneOwnedByCluster := newMachineBuilder().named("m3").ownedBy(&c).controlPlane().build()
	m4NotOwnedByCluster := newMachineBuilder().named("m4").build()
	m5OwnedByCluster := newMachineBuilder().named("m5").ownedBy(&c).build()
	m6ControlPlaneOwnedByCluster := newMachineBuilder().named("m6").ownedBy(&c).controlPlane().build()

	d := clusterDescendants{
		machineDeployments: clusterv1.MachineDeploymentList{
			Items: []clusterv1.MachineDeployment{
				md1NotOwnedByCluster,
				md2OwnedByCluster,
				md3NotOwnedByCluster,
				md4OwnedByCluster,
			},
		},
		machineSets: clusterv1.MachineSetList{
			Items: []clusterv1.MachineSet{
				ms1NotOwnedByCluster,
				ms2OwnedByCluster,
				ms3NotOwnedByCluster,
				ms4OwnedByCluster,
			},
		},
		controlPlaneMachines: clusterv1.MachineList{
			Items: []clusterv1.Machine{
				m3ControlPlaneOwnedByCluster,
				m6ControlPlaneOwnedByCluster,
			},
		},
		workerMachines: clusterv1.MachineList{
			Items: []clusterv1.Machine{
				m1NotOwnedByCluster,
				m2OwnedByCluster,
				m4NotOwnedByCluster,
				m5OwnedByCluster,
			},
		},
	}

	actual, err := d.filterOwnedDescendants(&c)
	g.Expect(err).NotTo(HaveOccurred())

	expected := []runtime.Object{
		&md2OwnedByCluster,
		&md4OwnedByCluster,
		&ms2OwnedByCluster,
		&ms4OwnedByCluster,
		&m2OwnedByCluster,
		&m5OwnedByCluster,
		&m3ControlPlaneOwnedByCluster,
		&m6ControlPlaneOwnedByCluster,
	}

	g.Expect(actual).To(Equal(expected))
}

func TestReconcileControlPlaneInitializedControlPlaneRef(t *testing.T) {
	g := NewWithT(t)

	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				APIVersion: "test.io/v1",
				Namespace:  "test",
				Name:       "foo",
			},
		},
	}

	r := &ClusterReconciler{
		Log: log.Log,
	}
	res, err := r.reconcileControlPlaneInitialized(context.Background(), c)
	g.Expect(res.IsZero()).To(BeTrue())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.Status.ControlPlaneInitialized).To(BeFalse())
}
