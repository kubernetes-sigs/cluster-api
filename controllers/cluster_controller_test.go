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
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Cluster Reconciler", func() {

	It("Should create a Cluster", func() {
		instance := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "foo-",
				Namespace:    "default",
			},
			Spec: clusterv1.ClusterSpec{},
		}

		// Create the Cluster object and expect the Reconcile and Deployment to be created
		Expect(k8sClient.Create(ctx, instance)).ToNot(HaveOccurred())
		key := client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}
		defer k8sClient.Delete(ctx, instance)

		// Make sure the Cluster exists.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, instance); err != nil {
				return false
			}
			return len(instance.Finalizers) > 0
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if the status diff is empty but the spec diff is not", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    "default",
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer k8sClient.Delete(ctx, cluster)

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, k8sClient)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Spec.InfrastructureRef = &v1.ObjectReference{Name: "test"}
			Expect(ph.Patch(ctx, cluster)).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := k8sClient.Get(ctx, key, instance); err != nil {
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
				GenerateName: "test-",
				Namespace:    "default",
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer k8sClient.Delete(ctx, cluster)

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, k8sClient)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Status.InfrastructureReady = true
			Expect(ph.Patch(ctx, cluster)).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := k8sClient.Get(ctx, key, instance); err != nil {
				return false
			}
			return instance.Status.InfrastructureReady
		}, timeout).Should(BeTrue())
	})

	It("Should successfully patch a cluster object if both the spec diff and status diff are non empty", func() {
		// Setup
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    "default",
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer k8sClient.Delete(ctx, cluster)

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, k8sClient)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.Status.InfrastructureReady = true
			cluster.Spec.InfrastructureRef = &v1.ObjectReference{Name: "test"}
			Expect(ph.Patch(ctx, cluster)).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		// Assertions
		Eventually(func() bool {
			instance := &clusterv1.Cluster{}
			if err := k8sClient.Get(ctx, key, instance); err != nil {
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
				GenerateName: "test-",
				Namespace:    "default",
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer k8sClient.Delete(ctx, cluster)

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// Patch
		Eventually(func() bool {
			ph, err := patch.NewHelper(cluster, k8sClient)
			Expect(err).ShouldNot(HaveOccurred())
			cluster.SetFinalizers([]string{})
			Expect(ph.Patch(ctx, cluster)).ShouldNot(HaveOccurred())
			return true
		}, timeout).Should(BeTrue())

		Expect(cluster.Finalizers).Should(BeEmpty())

		// Assertions
		Eventually(func() []string {
			instance := &clusterv1.Cluster{}
			if err := k8sClient.Get(ctx, key, instance); err != nil {
				return []string{"not-empty"}
			}
			return instance.Finalizers
		}, timeout).Should(BeEmpty())
	})

	It("Should successfully set Status.ControlPlaneInitialized on the cluster object if controlplane is ready", func() {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    v1.NamespaceDefault,
			},
		}

		Expect(k8sClient.Create(ctx, cluster)).To(BeNil())
		key := client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		defer k8sClient.Delete(ctx, cluster)

		// Wait for reconciliation to happen.
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return len(cluster.Finalizers) > 0
		}, timeout).Should(BeTrue())

		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    v1.NamespaceDefault,
				Labels: map[string]string{
					clusterv1.MachineControlPlaneLabelName: cluster.Name,
					clusterv1.MachineClusterLabelName:      cluster.Name,
				},
			},
			Spec: clusterv1.MachineSpec{
				ProviderID: pointer.StringPtr("aws:///id-node-1"),
				Bootstrap: clusterv1.Bootstrap{
					Data: pointer.StringPtr(""),
				},
			},
		}

		Expect(k8sClient.Create(ctx, machine)).To(BeNil())
		key = client.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}
		defer k8sClient.Delete(ctx, machine)

		// wait for machine to be ready
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, machine); err != nil {
				return false
			}
			return len(machine.Finalizers) > 0
		}, timeout).Should(BeTrue())

		// patch machine noderef
		patchHelper, err := patch.NewHelper(machine, k8sClient)
		Expect(err).NotTo(HaveOccurred())

		machine.Status.NodeRef = &v1.ObjectReference{Kind: "Node", Name: "test-node"}
		Expect(patchHelper.Patch(ctx, machine)).Should(BeNil())

		// Assertion
		key = client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, key, cluster); err != nil {
				return false
			}
			return cluster.Status.ControlPlaneInitialized
		}, timeout).Should(BeTrue())
	})
})

func TestClusterReconciler_machineToCluster(t *testing.T) {
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
			Name: "controlPlaneWithNoderef",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName:      cluster.Name,
				clusterv1.MachineControlPlaneLabelName: cluster.Name,
			},
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
			Name: "controlPlaneWithoutNoderef",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName:      cluster.Name,
				clusterv1.MachineControlPlaneLabelName: cluster.Name,
			},
		},
	}
	nonControlPlaneWithNoderef := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "nonControlPlaneWitNoderef",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: cluster.Name,
			},
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
			Name: "nonControlPlaneWithoutNoderef",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: cluster.Name,
			},
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
				{NamespacedName: client.ObjectKey{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				}},
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
			r := &ClusterReconciler{
				Client: fake.NewFakeClient(cluster, controlPlaneWithNoderef, controlPlaneWithoutNoderef, nonControlPlaneWithNoderef, nonControlPlaneWithoutNoderef),
				Log:    log.Log,
			}
			if got := r.controlPlaneMachineToCluster(tt.o); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controlPlaneMachineToCluster() = %v, want %v", got, tt.want)
			}
		})
	}
}
