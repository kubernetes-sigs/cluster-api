/*
Copyright 2023 The Kubernetes Authors.

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
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
	secretutil "sigs.k8s.io/cluster-api/util/secret"
)

var (
	ctx    = context.Background()
	scheme = runtime.NewScheme()

	cluster = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}

	cpMachine = &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
	}

	workerMachine = &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "baz",
		},
	}
)

func init() {
	_ = metav1.AddMetaToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = cloudv1.AddToScheme(scheme)

	ctrl.SetLogger(klog.Background())
}

func TestReconcileNormalCloudMachine(t *testing.T) {
	inMemoryMachine := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: infrav1.InMemoryMachineSpec{
			Behaviour: &infrav1.InMemoryMachineBehaviour{
				VM: &infrav1.InMemoryVMBehaviour{
					Provisioning: infrav1.CommonProvisioningSettings{
						StartupDuration: metav1.Duration{Duration: 2 * time.Second},
					},
				},
			},
		},
	}

	t.Run("create CloudMachine", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalCloudMachine(ctx, cluster, cpMachine, inMemoryMachine)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeFalse())
		g.Expect(conditions.IsFalse(inMemoryMachine, infrav1.VMProvisionedCondition)).To(BeTrue())

		got := &cloudv1.CloudMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: inMemoryMachine.Name,
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(err).ToNot(HaveOccurred())

		t.Run("gets provisioned after the provisioning time is expired", func(t *testing.T) {
			g := NewWithT(t)

			g.Eventually(func() bool {
				res, err := r.reconcileNormalCloudMachine(ctx, cluster, cpMachine, inMemoryMachine)
				g.Expect(err).ToNot(HaveOccurred())
				if !res.IsZero() {
					time.Sleep(res.RequeueAfter / 100 * 90)
				}
				return res.IsZero()
			}, inMemoryMachine.Spec.Behaviour.VM.Provisioning.StartupDuration.Duration*2).Should(BeTrue())

			g.Expect(conditions.IsTrue(inMemoryMachine, infrav1.VMProvisionedCondition)).To(BeTrue())
			g.Expect(conditions.Get(inMemoryMachine, infrav1.VMProvisionedCondition).LastTransitionTime.Time).To(BeTemporally(">", inMemoryMachine.CreationTimestamp.Time, inMemoryMachine.Spec.Behaviour.VM.Provisioning.StartupDuration.Duration))
		})

		t.Run("no-op after it is provisioned", func(t *testing.T) {
			g := NewWithT(t)

			res, err := r.reconcileNormalCloudMachine(ctx, cluster, cpMachine, inMemoryMachine)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res.IsZero()).To(BeTrue())
		})
	})
}

func TestReconcileNormalNode(t *testing.T) {
	inMemoryMachineWithVMNotYetProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
	}

	inMemoryMachineWithVMProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: infrav1.InMemoryMachineSpec{
			Behaviour: &infrav1.InMemoryMachineBehaviour{
				Node: &infrav1.InMemoryNodeBehaviour{
					Provisioning: infrav1.CommonProvisioningSettings{
						StartupDuration: metav1.Duration{Duration: 2 * time.Second},
					},
				},
			},
		},
		Status: infrav1.InMemoryMachineStatus{
			Conditions: []clusterv1.Condition{
				{
					Type:               infrav1.VMProvisionedCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	t.Run("no-op if VM is not yet ready", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalNode(ctx, cluster, cpMachine, inMemoryMachineWithVMNotYetProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: inMemoryMachineWithVMNotYetProvisioned.Name,
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("create node if VM is ready", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalNode(ctx, cluster, cpMachine, inMemoryMachineWithVMProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeFalse())
		g.Expect(conditions.IsFalse(inMemoryMachineWithVMProvisioned, infrav1.NodeProvisionedCondition)).To(BeTrue())

		got := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: inMemoryMachineWithVMProvisioned.Name,
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

		t.Run("gets provisioned after the provisioning time is expired", func(t *testing.T) {
			g := NewWithT(t)

			g.Eventually(func() bool {
				res, err := r.reconcileNormalNode(ctx, cluster, cpMachine, inMemoryMachineWithVMProvisioned)
				g.Expect(err).ToNot(HaveOccurred())
				if !res.IsZero() {
					time.Sleep(res.RequeueAfter / 100 * 90)
				}
				return res.IsZero()
			}, inMemoryMachineWithVMProvisioned.Spec.Behaviour.Node.Provisioning.StartupDuration.Duration*2).Should(BeTrue())

			err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(conditions.IsTrue(inMemoryMachineWithVMProvisioned, infrav1.NodeProvisionedCondition)).To(BeTrue())
			g.Expect(conditions.Get(inMemoryMachineWithVMProvisioned, infrav1.NodeProvisionedCondition).LastTransitionTime.Time).To(BeTemporally(">", conditions.Get(inMemoryMachineWithVMProvisioned, infrav1.VMProvisionedCondition).LastTransitionTime.Time, inMemoryMachineWithVMProvisioned.Spec.Behaviour.Node.Provisioning.StartupDuration.Duration))
		})

		t.Run("no-op after it is provisioned", func(t *testing.T) {
			g := NewWithT(t)

			res, err := r.reconcileNormalNode(ctx, cluster, cpMachine, inMemoryMachineWithVMProvisioned)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res.IsZero()).To(BeTrue())
		})
	})
}

func TestReconcileNormalEtcd(t *testing.T) {
	inMemoryMachineWithNodeNotYetProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar0",
		},
	}

	inMemoryMachineWithNodeProvisioned1 := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar1",
		},
		Spec: infrav1.InMemoryMachineSpec{
			Behaviour: &infrav1.InMemoryMachineBehaviour{
				Etcd: &infrav1.InMemoryEtcdBehaviour{
					Provisioning: infrav1.CommonProvisioningSettings{
						StartupDuration: metav1.Duration{Duration: 2 * time.Second},
					},
				},
			},
		},
		Status: infrav1.InMemoryMachineStatus{
			Conditions: []clusterv1.Condition{
				{
					Type:               infrav1.NodeProvisionedCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	t.Run("no-op for worker machines", func(*testing.T) {
		// TODO: implement test
	})

	t.Run("no-op if Node is not yet ready", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeNotYetProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("etcd-%s", inMemoryMachineWithNodeNotYetProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("create pod if Node is ready", func(t *testing.T) {
		g := NewWithT(t)

		manager := inmemoryruntime.NewManager(scheme)

		host := "127.0.0.1"
		wcmux, err := inmemoryserver.NewWorkloadClustersMux(manager, host, inmemoryserver.CustomPorts{
			// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
			MinPort:   inmemoryserver.DefaultMinPort + 1000,
			MaxPort:   inmemoryserver.DefaultMinPort + 1099,
			DebugPort: inmemoryserver.DefaultDebugPort + 10,
		})
		g.Expect(err).ToNot(HaveOccurred())
		_, err = wcmux.InitWorkloadClusterListener(klog.KObj(cluster).String())
		g.Expect(err).ToNot(HaveOccurred())

		r := InMemoryMachineReconciler{
			Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(createCASecret(t, cluster, secretutil.EtcdCA)).Build(),
			InMemoryManager: manager,
			APIServerMux:    wcmux,
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeFalse())
		g.Expect(conditions.IsFalse(inMemoryMachineWithNodeProvisioned1, infrav1.EtcdProvisionedCondition)).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("etcd-%s", inMemoryMachineWithNodeProvisioned1.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

		t.Run("gets provisioned after the provisioning time is expired", func(t *testing.T) {
			g := NewWithT(t)

			g.Eventually(func() bool {
				res, err := r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned1)
				g.Expect(err).ToNot(HaveOccurred())
				if !res.IsZero() {
					time.Sleep(res.RequeueAfter / 100 * 90)
				}
				return res.IsZero()
			}, inMemoryMachineWithNodeProvisioned1.Spec.Behaviour.Etcd.Provisioning.StartupDuration.Duration*2).Should(BeTrue())

			err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got.Annotations).To(HaveKey(cloudv1.EtcdClusterIDAnnotationName))
			g.Expect(got.Annotations).To(HaveKey(cloudv1.EtcdMemberIDAnnotationName))
			g.Expect(got.Annotations).To(HaveKey(cloudv1.EtcdLeaderFromAnnotationName))

			g.Expect(conditions.IsTrue(inMemoryMachineWithNodeProvisioned1, infrav1.EtcdProvisionedCondition)).To(BeTrue())
			g.Expect(conditions.Get(inMemoryMachineWithNodeProvisioned1, infrav1.EtcdProvisionedCondition).LastTransitionTime.Time).To(BeTemporally(">", conditions.Get(inMemoryMachineWithNodeProvisioned1, infrav1.NodeProvisionedCondition).LastTransitionTime.Time, inMemoryMachineWithNodeProvisioned1.Spec.Behaviour.Etcd.Provisioning.StartupDuration.Duration))
		})

		t.Run("no-op after it is provisioned", func(t *testing.T) {
			g := NewWithT(t)

			res, err := r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned1)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res.IsZero()).To(BeTrue())
		})

		err = wcmux.Shutdown(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("takes care of the etcd cluster annotations", func(t *testing.T) {
		g := NewWithT(t)

		inMemoryMachineWithNodeProvisioned1 := inMemoryMachineWithNodeProvisioned1.DeepCopy()
		inMemoryMachineWithNodeProvisioned1.Spec = infrav1.InMemoryMachineSpec{}

		inMemoryMachineWithNodeProvisioned2 := inMemoryMachineWithNodeProvisioned1.DeepCopy()
		inMemoryMachineWithNodeProvisioned2.Name = "bar2"

		manager := inmemoryruntime.NewManager(scheme)

		host := "127.0.0.1"
		wcmux, err := inmemoryserver.NewWorkloadClustersMux(manager, host, inmemoryserver.CustomPorts{
			// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
			MinPort:   inmemoryserver.DefaultMinPort + 1200,
			MaxPort:   inmemoryserver.DefaultMinPort + 1299,
			DebugPort: inmemoryserver.DefaultDebugPort + 20,
		})
		g.Expect(err).ToNot(HaveOccurred())
		_, err = wcmux.InitWorkloadClusterListener(klog.KObj(cluster).String())
		g.Expect(err).ToNot(HaveOccurred())

		r := InMemoryMachineReconciler{
			Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(createCASecret(t, cluster, secretutil.EtcdCA)).Build(),
			InMemoryManager: manager,
			APIServerMux:    wcmux,
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		// first etcd pod gets annotated with clusterID, memberID, and also set as a leader

		res, err := r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(conditions.IsTrue(inMemoryMachineWithNodeProvisioned1, infrav1.EtcdProvisionedCondition)).To(BeTrue())

		got1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("etcd-%s", inMemoryMachineWithNodeProvisioned1.Name),
			},
		}

		err = c.Get(ctx, client.ObjectKeyFromObject(got1), got1)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(got1.Annotations).To(HaveKey(cloudv1.EtcdClusterIDAnnotationName))
		g.Expect(got1.Annotations).To(HaveKey(cloudv1.EtcdMemberIDAnnotationName))
		g.Expect(got1.Annotations).To(HaveKey(cloudv1.EtcdLeaderFromAnnotationName))

		// second etcd pod gets annotated with the same clusterID, a new memberID (but it is not set as a leader

		res, err = r.reconcileNormalETCD(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned2)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())
		g.Expect(conditions.IsTrue(inMemoryMachineWithNodeProvisioned2, infrav1.EtcdProvisionedCondition)).To(BeTrue())

		got2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("etcd-%s", inMemoryMachineWithNodeProvisioned2.Name),
			},
		}

		err = c.Get(ctx, client.ObjectKeyFromObject(got2), got2)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(got2.Annotations).To(HaveKey(cloudv1.EtcdClusterIDAnnotationName))
		g.Expect(got1.Annotations[cloudv1.EtcdClusterIDAnnotationName]).To(Equal(got2.Annotations[cloudv1.EtcdClusterIDAnnotationName]))
		g.Expect(got2.Annotations).To(HaveKey(cloudv1.EtcdMemberIDAnnotationName))
		g.Expect(got1.Annotations[cloudv1.EtcdMemberIDAnnotationName]).ToNot(Equal(got2.Annotations[cloudv1.EtcdMemberIDAnnotationName]))
		g.Expect(got2.Annotations).ToNot(HaveKey(cloudv1.EtcdLeaderFromAnnotationName))
	})
}

func TestReconcileNormalApiServer(t *testing.T) {
	inMemoryMachineWithNodeNotYetProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
	}

	inMemoryMachineWithNodeProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Spec: infrav1.InMemoryMachineSpec{
			Behaviour: &infrav1.InMemoryMachineBehaviour{
				APIServer: &infrav1.InMemoryAPIServerBehaviour{
					Provisioning: infrav1.CommonProvisioningSettings{
						StartupDuration: metav1.Duration{Duration: 2 * time.Second},
					},
				},
			},
		},
		Status: infrav1.InMemoryMachineStatus{
			Conditions: []clusterv1.Condition{
				{
					Type:               infrav1.NodeProvisionedCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	t.Run("no-op for worker machines", func(*testing.T) {
		// TODO: implement test
	})

	t.Run("no-op if Node is not yet ready", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalAPIServer(ctx, cluster, cpMachine, inMemoryMachineWithNodeNotYetProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("kube-apiserver-%s", inMemoryMachineWithNodeNotYetProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("create pod if Node is ready", func(t *testing.T) {
		g := NewWithT(t)

		manager := inmemoryruntime.NewManager(scheme)

		host := "127.0.0.1"
		wcmux, err := inmemoryserver.NewWorkloadClustersMux(manager, host, inmemoryserver.CustomPorts{
			// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
			MinPort:   inmemoryserver.DefaultMinPort + 1100,
			MaxPort:   inmemoryserver.DefaultMinPort + 1199,
			DebugPort: inmemoryserver.DefaultDebugPort + 11,
		})
		g.Expect(err).ToNot(HaveOccurred())
		_, err = wcmux.InitWorkloadClusterListener(klog.KObj(cluster).String())
		g.Expect(err).ToNot(HaveOccurred())

		r := InMemoryMachineReconciler{
			Client:          fake.NewClientBuilder().WithScheme(scheme).WithObjects(createCASecret(t, cluster, secretutil.ClusterCA)).Build(),
			InMemoryManager: manager,
			APIServerMux:    wcmux,
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := r.reconcileNormalAPIServer(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeFalse())
		g.Expect(conditions.IsFalse(inMemoryMachineWithNodeProvisioned, infrav1.APIServerProvisionedCondition)).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("kube-apiserver-%s", inMemoryMachineWithNodeNotYetProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

		t.Run("gets provisioned after the provisioning time is expired", func(t *testing.T) {
			g := NewWithT(t)

			g.Eventually(func() bool {
				res, err := r.reconcileNormalAPIServer(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned)
				g.Expect(err).ToNot(HaveOccurred())
				if !res.IsZero() {
					time.Sleep(res.RequeueAfter / 100 * 90)
				}
				return res.IsZero()
			}, inMemoryMachineWithNodeProvisioned.Spec.Behaviour.APIServer.Provisioning.StartupDuration.Duration*2).Should(BeTrue())

			err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(conditions.IsTrue(inMemoryMachineWithNodeProvisioned, infrav1.APIServerProvisionedCondition)).To(BeTrue())
			g.Expect(conditions.Get(inMemoryMachineWithNodeProvisioned, infrav1.APIServerProvisionedCondition).LastTransitionTime.Time).To(BeTemporally(">", conditions.Get(inMemoryMachineWithNodeProvisioned, infrav1.NodeProvisionedCondition).LastTransitionTime.Time, inMemoryMachineWithNodeProvisioned.Spec.Behaviour.APIServer.Provisioning.StartupDuration.Duration))
		})

		t.Run("no-op after it is provisioned", func(t *testing.T) {
			g := NewWithT(t)

			res, err := r.reconcileNormalAPIServer(ctx, cluster, cpMachine, inMemoryMachineWithNodeProvisioned)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res.IsZero()).To(BeTrue())
		})

		err = wcmux.Shutdown(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestReconcileNormalScheduler(t *testing.T) {
	testReconcileNormalComponent(t, "kube-scheduler", func(r InMemoryMachineReconciler) func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
		return r.reconcileNormalScheduler
	})
}

func TestReconcileNormalControllerManager(t *testing.T) {
	testReconcileNormalComponent(t, "kube-controller-manager", func(r InMemoryMachineReconciler) func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error) {
		return r.reconcileNormalControllerManager
	})
}

func testReconcileNormalComponent(t *testing.T, component string, reconcileFunc func(InMemoryMachineReconciler) func(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, inMemoryMachine *infrav1.InMemoryMachine) (ctrl.Result, error)) {
	t.Helper()

	inMemoryMachineWithAPIServerNotYetProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
	}

	inMemoryMachineWithAPIServerProvisioned := &infrav1.InMemoryMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Status: infrav1.InMemoryMachineStatus{
			Conditions: []clusterv1.Condition{
				{
					Type:   infrav1.APIServerProvisionedCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	t.Run("no-op for worker machines", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := reconcileFunc(r)(ctx, cluster, workerMachine, inMemoryMachineWithAPIServerProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("%s-%s", component, inMemoryMachineWithAPIServerProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("no-op if API server is not yet ready", func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := reconcileFunc(r)(ctx, cluster, cpMachine, inMemoryMachineWithAPIServerNotYetProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("%s-%s", component, inMemoryMachineWithAPIServerProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run(fmt.Sprintf("create %s pod if API server is ready", component), func(t *testing.T) {
		g := NewWithT(t)

		r := InMemoryMachineReconciler{
			InMemoryManager: inmemoryruntime.NewManager(scheme),
		}
		r.InMemoryManager.AddResourceGroup(klog.KObj(cluster).String())
		c := r.InMemoryManager.GetResourceGroup(klog.KObj(cluster).String()).GetClient()

		res, err := reconcileFunc(r)(ctx, cluster, cpMachine, inMemoryMachineWithAPIServerProvisioned)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.IsZero()).To(BeTrue())

		got := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      fmt.Sprintf("%s-%s", component, inMemoryMachineWithAPIServerProvisioned.Name),
			},
		}
		err = c.Get(ctx, client.ObjectKeyFromObject(got), got)
		g.Expect(err).ToNot(HaveOccurred())

		t.Run(fmt.Sprintf("no-op if %s pod already exists", component), func(t *testing.T) {
			g := NewWithT(t)

			res, err := reconcileFunc(r)(ctx, cluster, cpMachine, inMemoryMachineWithAPIServerProvisioned)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res.IsZero()).To(BeTrue())
		})
	})
}

func createCASecret(t *testing.T, cluster *clusterv1.Cluster, purpose secretutil.Purpose) *corev1.Secret {
	t.Helper()

	g := NewWithT(t)

	cert, key, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      secretutil.Name(cluster.Name, purpose),
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name,
			},
		},
		Data: map[string][]byte{
			secretutil.TLSKeyDataName: certs.EncodePrivateKeyPEM(key),
			secretutil.TLSCrtDataName: certs.EncodeCertPEM(cert),
		},
		Type: clusterv1.ClusterSecretType,
	}
}

// TODO: make this public functions in server/certs.go or in a new util package.

// newCertificateAuthority creates new certificate and private key for the certificate authority.
func newCertificateAuthority() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := certs.NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	c, err := newSelfSignedCACert(key)
	if err != nil {
		return nil, nil, err
	}

	return c, key, nil
}

// newSelfSignedCACert creates a CA certificate.
func newSelfSignedCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.Add(time.Minute * -5),
		NotAfter:              now.Add(time.Hour * 24 * 365 * 10), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create self signed CA certificate: %+v", tmpl)
	}

	c, err := x509.ParseCertificate(b)
	return c, errors.WithStack(err)
}
