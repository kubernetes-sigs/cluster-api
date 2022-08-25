/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge"
	"sigs.k8s.io/cluster-api/internal/hooks"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	. "sigs.k8s.io/cluster-api/internal/test/matchers"
)

var (
	IgnoreNameGenerated = IgnorePaths{
		{"metadata", "name"},
	}
)

func TestReconcileShim(t *testing.T) {
	infrastructureCluster := builder.TestInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").Build()
	controlPlane := builder.TestControlPlane(metav1.NamespaceDefault, "controlplane-cluster1").Build()
	cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").Build()
	// cluster requires a UID because reconcileClusterShim will create a cluster shim
	// which has the cluster set as Owner in an OwnerReference.
	// A valid OwnerReferences requires a uid.
	cluster.SetUID("foo")

	t.Run("Shim gets created when InfrastructureCluster and ControlPlane object have to be created", func(t *testing.T) {
		g := NewWithT(t)

		// Create namespace and modify input to have correct namespace set
		namespace, err := env.CreateNamespace(ctx, "reconcile-cluster-shim")
		g.Expect(err).ToNot(HaveOccurred())
		cluster1 := cluster.DeepCopy()
		cluster1.SetNamespace(namespace.GetName())
		cluster1Shim := clusterShim(cluster1)

		// Create a scope with a cluster and InfrastructureCluster yet to be created.
		s := scope.New(cluster1)
		s.Desired = &scope.ClusterState{
			InfrastructureCluster: infrastructureCluster.DeepCopy(),
			ControlPlane: &scope.ControlPlaneState{
				Object: controlPlane.DeepCopy(),
			},
		}

		// Run reconcileClusterShim.
		r := Reconciler{
			Client:             env,
			APIReader:          env.GetAPIReader(),
			patchHelperFactory: serverSideApplyPatchHelperFactory(env),
		}
		err = r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())

		// Check shim is assigned as owner for InfrastructureCluster and ControlPlane objects.
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
		g.Expect(env.CleanupAndWait(ctx, cluster1Shim)).To(Succeed())
	})
	t.Run("Shim creation is re-entrant", func(t *testing.T) {
		g := NewWithT(t)

		// Create namespace and modify input to have correct namespace set
		namespace, err := env.CreateNamespace(ctx, "reconcile-cluster-shim")
		g.Expect(err).ToNot(HaveOccurred())
		cluster1 := cluster.DeepCopy()
		cluster1.SetNamespace(namespace.GetName())
		cluster1Shim := clusterShim(cluster1)

		// Create a scope with a cluster and InfrastructureCluster yet to be created.
		s := scope.New(cluster1)
		s.Desired = &scope.ClusterState{
			InfrastructureCluster: infrastructureCluster.DeepCopy(),
			ControlPlane: &scope.ControlPlaneState{
				Object: controlPlane.DeepCopy(),
			},
		}

		// Pre-create a shim
		g.Expect(env.CreateAndWait(ctx, cluster1Shim.DeepCopy())).ToNot(HaveOccurred())

		// Run reconcileClusterShim.
		r := Reconciler{
			Client:             env,
			APIReader:          env.GetAPIReader(),
			patchHelperFactory: serverSideApplyPatchHelperFactory(env),
		}
		err = r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())

		// Check shim is assigned as owner for InfrastructureCluster and ControlPlane objects.
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()[0].Name).To(Equal(shim.Name))

		g.Expect(env.CleanupAndWait(ctx, cluster1Shim)).To(Succeed())
	})
	t.Run("Shim is not deleted if InfrastructureCluster and ControlPlane object are waiting to be reconciled", func(t *testing.T) {
		g := NewWithT(t)

		// Create namespace and modify input to have correct namespace set
		namespace, err := env.CreateNamespace(ctx, "reconcile-cluster-shim")
		g.Expect(err).ToNot(HaveOccurred())
		cluster1 := cluster.DeepCopy()
		cluster1.SetNamespace(namespace.GetName())
		cluster1Shim := clusterShim(cluster1)

		// Create a scope with a cluster and InfrastructureCluster created but not yet reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the shim as a temporary owner for the InfrastructureCluster and ControlPlane.
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Pre-create a shim
		g.Expect(env.CreateAndWait(ctx, cluster1Shim.DeepCopy())).ToNot(HaveOccurred())

		// Run reconcileClusterShim.
		r := Reconciler{
			Client:             env,
			APIReader:          env.GetAPIReader(),
			patchHelperFactory: serverSideApplyPatchHelperFactory(env),
		}
		err = r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.CleanupAndWait(ctx, cluster1Shim)).To(Succeed())
	})
	t.Run("Shim gets deleted when InfrastructureCluster and ControlPlane object have been reconciled", func(t *testing.T) {
		g := NewWithT(t)

		// Create namespace and modify input to have correct namespace set
		namespace, err := env.CreateNamespace(ctx, "reconcile-cluster-shim")
		g.Expect(err).ToNot(HaveOccurred())
		cluster1 := cluster.DeepCopy()
		cluster1.SetNamespace(namespace.GetName())
		cluster1Shim := clusterShim(cluster1)

		// Create a scope with a cluster and InfrastructureCluster created and reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the shim as a temporary owner for the InfrastructureCluster and ControlPlane.
		// Add the cluster as a final owner for the InfrastructureCluster and ControlPlane (reconciled).
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Pre-create a shim
		g.Expect(env.CreateAndWait(ctx, cluster1Shim.DeepCopy())).ToNot(HaveOccurred())

		// Run reconcileClusterShim.
		r := Reconciler{
			Client:             env,
			APIReader:          env.GetAPIReader(),
			patchHelperFactory: serverSideApplyPatchHelperFactory(env),
		}
		err = r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

		g.Expect(env.CleanupAndWait(ctx, cluster1Shim)).To(Succeed())
	})
	t.Run("No op if InfrastructureCluster and ControlPlane object have been reconciled and shim is gone", func(t *testing.T) {
		g := NewWithT(t)

		// Create namespace and modify input to have correct namespace set
		namespace, err := env.CreateNamespace(ctx, "reconcile-cluster-shim")
		g.Expect(err).ToNot(HaveOccurred())
		cluster1 := cluster.DeepCopy()
		cluster1.SetNamespace(namespace.GetName())
		cluster1Shim := clusterShim(cluster1)

		// Create a scope with a cluster and InfrastructureCluster created and reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the cluster as a final owner for the InfrastructureCluster and ControlPlane (reconciled).
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Run reconcileClusterShim using a nil client, so an error will be triggered if any operation is attempted
		r := Reconciler{
			Client:             nil,
			APIReader:          env.GetAPIReader(),
			patchHelperFactory: serverSideApplyPatchHelperFactory(nil),
		}
		err = r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.CleanupAndWait(ctx, cluster1Shim)).To(Succeed())
	})
}

func TestReconcile_callAfterControlPlaneInitialized(t *testing.T) {
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	afterControlPlaneInitializedGVH, err := catalog.GroupVersionHook(runtimehooksv1.AfterControlPlaneInitialized)
	if err != nil {
		panic(err)
	}

	successResponse := &runtimehooksv1.AfterControlPlaneInitializedResponse{

		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusSuccess,
		},
	}
	failureResponse := &runtimehooksv1.AfterControlPlaneInitializedResponse{
		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusFailure,
		},
	}

	tests := []struct {
		name               string
		cluster            *clusterv1.Cluster
		hookResponse       *runtimehooksv1.AfterControlPlaneInitializedResponse
		wantMarked         bool
		wantHookToBeCalled bool
		wantError          bool
	}{
		{
			name: "hook should be marked if the cluster is about to be created",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: clusterv1.ClusterSpec{},
			},
			hookResponse:       successResponse,
			wantMarked:         true,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should be called if it is marked and the control plane is ready - the hook should become unmarked for a success response",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimev1.PendingHooksAnnotation: "AfterControlPlaneInitialized",
					},
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef:   &corev1.ObjectReference{},
					InfrastructureRef: &corev1.ObjectReference{},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{
							Type:   clusterv1.ControlPlaneInitializedCondition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			hookResponse:       successResponse,
			wantMarked:         false,
			wantHookToBeCalled: true,
			wantError:          false,
		},
		{
			name: "hook should be called if it is marked and the control plane is ready - the hook should remain marked for a failure response",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimev1.PendingHooksAnnotation: "AfterControlPlaneInitialized",
					},
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef:   &corev1.ObjectReference{},
					InfrastructureRef: &corev1.ObjectReference{},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{
							Type:   clusterv1.ControlPlaneInitializedCondition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			hookResponse:       failureResponse,
			wantMarked:         true,
			wantHookToBeCalled: true,
			wantError:          true,
		},
		{
			name: "hook should not be called if it is marked and the control plane is not ready - the hook should remain marked",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimev1.PendingHooksAnnotation: "AfterControlPlaneInitialized",
					},
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef:   &corev1.ObjectReference{},
					InfrastructureRef: &corev1.ObjectReference{},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{
							Type:   clusterv1.ControlPlaneInitializedCondition,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			hookResponse:       failureResponse,
			wantMarked:         true,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if it is not marked",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef:   &corev1.ObjectReference{},
					InfrastructureRef: &corev1.ObjectReference{},
				},
				Status: clusterv1.ClusterStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{
							Type:   clusterv1.ControlPlaneInitializedCondition,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			hookResponse:       failureResponse,
			wantMarked:         false,
			wantHookToBeCalled: false,
			wantError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: tt.cluster,
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
			}

			fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					afterControlPlaneInitializedGVH: tt.hookResponse,
				}).
				WithCatalog(catalog).
				Build()

			fakeClient := fake.NewClientBuilder().WithObjects(tt.cluster).Build()

			r := &Reconciler{
				Client:        fakeClient,
				APIReader:     fakeClient,
				RuntimeClient: fakeRuntimeClient,
			}

			err := r.callAfterControlPlaneInitialized(ctx, s)
			g.Expect(fakeRuntimeClient.CallAllCount(runtimehooksv1.AfterControlPlaneInitialized) == 1).To(Equal(tt.wantHookToBeCalled))
			g.Expect(hooks.IsPending(runtimehooksv1.AfterControlPlaneInitialized, tt.cluster)).To(Equal(tt.wantMarked))
			g.Expect(err != nil).To(Equal(tt.wantError))
		})
	}
}

func TestReconcile_callAfterClusterUpgrade(t *testing.T) {
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	afterClusterUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.AfterClusterUpgrade)
	if err != nil {
		panic(err)
	}

	successResponse := &runtimehooksv1.AfterClusterUpgradeResponse{

		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusSuccess,
		},
	}
	failureResponse := &runtimehooksv1.AfterClusterUpgradeResponse{
		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusFailure,
		},
	}

	topologyVersion := "v1.2.3"
	lowerVersion := "v1.2.2"
	controlPlaneStableAtTopologyVersion := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  topologyVersion,
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         topologyVersion,
			"status.replicas":        int64(2),
			"status.updatedReplicas": int64(2),
			"status.readyReplicas":   int64(2),
		}).
		Build()
	controlPlaneStableAtLowerVersion := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  lowerVersion,
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         lowerVersion,
			"status.replicas":        int64(2),
			"status.updatedReplicas": int64(2),
			"status.readyReplicas":   int64(2),
		}).
		Build()
	controlPlaneUpgrading := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  topologyVersion,
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         lowerVersion,
			"status.replicas":        int64(2),
			"status.updatedReplicas": int64(2),
			"status.readyReplicas":   int64(2),
		}).
		Build()
	controlPlaneScaling := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  topologyVersion,
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         topologyVersion,
			"status.replicas":        int64(1),
			"status.updatedReplicas": int64(1),
			"status.readyReplicas":   int64(1),
		}).
		Build()

	tests := []struct {
		name               string
		s                  *scope.Scope
		hookResponse       *runtimehooksv1.AfterClusterUpgradeResponse
		wantMarked         bool
		wantHookToBeCalled bool
		wantError          bool
	}{
		{
			name: "hook should not be called if it is not marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
						},
						Spec: clusterv1.ClusterSpec{},
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker:      scope.NewUpgradeTracker(),
			},
			wantMarked:         false,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if the control plane is upgrading - hook is marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneUpgrading,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker:      scope.NewUpgradeTracker(),
			},
			wantMarked:         true,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if the control plane is scaling - hook is marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneScaling,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker:      scope.NewUpgradeTracker(),
			},
			wantMarked:         true,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if the control plane is stable at a lower version and is pending an upgrade - hook is marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneStableAtLowerVersion,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = true
					return ut
				}(),
			},
			wantMarked:         true,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if the control plane is stable at desired version but MDs are rolling out - hook is marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneStableAtTopologyVersion,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.MachineDeployments.MarkRollingOut("md1")
					return ut
				}(),
			},
			wantMarked:         true,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should not be called if the control plane is stable at desired version but MDs are pending upgrade - hook is marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneStableAtTopologyVersion,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					return ut
				}(),
			},
			wantMarked:         true,
			hookResponse:       successResponse,
			wantHookToBeCalled: false,
			wantError:          false,
		},
		{
			name: "hook should be called if the control plane and MDs are stable at the topology version - success response should unmark the hook",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{
							Topology: &clusterv1.Topology{
								Version: topologyVersion,
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneStableAtTopologyVersion,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker:      scope.NewUpgradeTracker(),
			},
			wantMarked:         false,
			hookResponse:       successResponse,
			wantHookToBeCalled: true,
			wantError:          false,
		},
		{
			name: "hook should be called if the control plane and MDs are stable at the topology version - failure response should leave the hook marked",
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						ControlPlane: clusterv1.ControlPlaneTopology{
							Replicas: pointer.Int32(2),
						},
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{
							Topology: &clusterv1.Topology{
								Version: topologyVersion,
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: controlPlaneStableAtTopologyVersion,
					},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
				UpgradeTracker:      scope.NewUpgradeTracker(),
			},
			wantMarked:         true,
			hookResponse:       failureResponse,
			wantHookToBeCalled: true,
			wantError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					afterClusterUpgradeGVH: tt.hookResponse,
				}).
				WithCatalog(catalog).
				Build()

			fakeClient := fake.NewClientBuilder().WithObjects(tt.s.Current.Cluster).Build()

			r := &Reconciler{
				Client:        fakeClient,
				APIReader:     fakeClient,
				RuntimeClient: fakeRuntimeClient,
			}

			err := r.callAfterClusterUpgrade(ctx, tt.s)
			g.Expect(fakeRuntimeClient.CallAllCount(runtimehooksv1.AfterClusterUpgrade) == 1).To(Equal(tt.wantHookToBeCalled))
			g.Expect(hooks.IsPending(runtimehooksv1.AfterClusterUpgrade, tt.s.Current.Cluster)).To(Equal(tt.wantMarked))
			g.Expect(err != nil).To(Equal(tt.wantError))
		})
	}
}

func TestReconcileCluster(t *testing.T) {
	cluster1 := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		Build()
	cluster1WithReferences := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		WithInfrastructureCluster(builder.TestInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").
			Build()).
		WithControlPlane(builder.TestControlPlane(metav1.NamespaceDefault, "control-plane1").Build()).
		Build()
	cluster2WithReferences := cluster1WithReferences.DeepCopy()
	cluster2WithReferences.SetGroupVersionKind(cluster1WithReferences.GroupVersionKind())
	cluster2WithReferences.Name = "cluster2"

	tests := []struct {
		name    string
		current *clusterv1.Cluster
		desired *clusterv1.Cluster
		want    *clusterv1.Cluster
		wantErr bool
	}{
		{
			name:    "Should update the cluster if infrastructure and control plane references are not set",
			current: cluster1,
			desired: cluster1WithReferences,
			want:    cluster1WithReferences,
			wantErr: false,
		},
		{
			name:    "Should be a no op if infrastructure and control plane references are already set",
			current: cluster2WithReferences,
			desired: cluster2WithReferences,
			want:    cluster2WithReferences,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-cluster")
			g.Expect(err).ToNot(HaveOccurred())
			if tt.desired != nil {
				tt.desired = prepareCluster(tt.desired, namespace.GetName())
			}
			if tt.want != nil {
				tt.want = prepareCluster(tt.want, namespace.GetName())
			}
			if tt.current != nil {
				tt.current = prepareCluster(tt.current, namespace.GetName())
			}

			if tt.current != nil {
				// NOTE: it is ok to use create given that the Cluster are created by user.
				g.Expect(env.CreateAndWait(ctx, tt.current)).To(Succeed())
			}

			s := scope.New(tt.current)

			s.Desired = &scope.ClusterState{Cluster: tt.desired}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}
			err = r.reconcileCluster(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy()
			err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got.Spec.InfrastructureRef).To(EqualObject(tt.want.Spec.InfrastructureRef))
			g.Expect(got.Spec.ControlPlaneRef).To(EqualObject(tt.want.Spec.ControlPlaneRef))

			if tt.current != nil {
				g.Expect(env.CleanupAndWait(ctx, tt.current)).To(Succeed())
			}
		})
	}
}

func TestReconcileInfrastructureCluster(t *testing.T) {
	g := NewWithT(t)

	// build an infrastructure cluster with a field managed by the topology controller (derived from the template).
	clusterInfrastructure1 := builder.TestInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").
		WithSpecFields(map[string]interface{}{"spec.foo": "foo"}).
		Build()

	// build a patch used to simulate instance specific changes made by an external controller, and build the expected cluster infrastructure object.
	clusterInfrastructure1ExternalChanges := "{ \"spec\": { \"bar\": \"bar\" }}"
	clusterInfrastructure1WithExternalChanges := clusterInfrastructure1.DeepCopy()
	g.Expect(unstructured.SetNestedField(clusterInfrastructure1WithExternalChanges.UnstructuredContent(), "bar", "spec", "bar")).To(Succeed())

	// build a patch used to simulate an external controller overriding a field managed by the topology controller.
	clusterInfrastructure1TemplateOverridingChanges := "{ \"spec\": { \"foo\": \"foo-override\" }}"

	// build a desired infrastructure cluster with incompatible changes.
	clusterInfrastructure1WithIncompatibleChanges := clusterInfrastructure1.DeepCopy()
	clusterInfrastructure1WithIncompatibleChanges.SetName("infrastructure-cluster1-changed")

	tests := []struct {
		name            string
		original        *unstructured.Unstructured
		externalChanges string
		desired         *unstructured.Unstructured
		want            *unstructured.Unstructured
		wantErr         bool
	}{
		{
			name:     "Should create desired InfrastructureCluster if the current does not exists yet",
			original: nil,
			desired:  clusterInfrastructure1,
			want:     clusterInfrastructure1,
			wantErr:  false,
		},
		{
			name:     "No-op if current InfrastructureCluster is equal to desired",
			original: clusterInfrastructure1,
			desired:  clusterInfrastructure1,
			want:     clusterInfrastructure1,
			wantErr:  false,
		},
		{
			name:            "Should preserve changes from external controllers",
			original:        clusterInfrastructure1,
			externalChanges: clusterInfrastructure1ExternalChanges,
			desired:         clusterInfrastructure1,
			want:            clusterInfrastructure1WithExternalChanges,
			wantErr:         false,
		},
		{
			name:            "Should restore template values if overridden by external controllers",
			original:        clusterInfrastructure1,
			externalChanges: clusterInfrastructure1TemplateOverridingChanges,
			desired:         clusterInfrastructure1,
			want:            clusterInfrastructure1,
			wantErr:         false,
		},
		{
			name:     "Fails for incompatible changes",
			original: clusterInfrastructure1,
			desired:  clusterInfrastructure1WithIncompatibleChanges,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-infrastructure-cluster")
			g.Expect(err).ToNot(HaveOccurred())
			if tt.original != nil {
				tt.original.SetNamespace(namespace.GetName())
			}
			if tt.desired != nil {
				tt.desired.SetNamespace(namespace.GetName())
			}
			if tt.want != nil {
				tt.want.SetNamespace(namespace.GetName())
			}

			if tt.original != nil {
				// NOTE: it is required to use server side apply to creat the object in order to ensure consistency with the topology controller behaviour.
				g.Expect(env.PatchAndWait(ctx, tt.original.DeepCopy(), client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				// NOTE: it is required to apply instance specific changes with a "plain" Patch operation to simulate a different manger.
				if tt.externalChanges != "" {
					g.Expect(env.Patch(ctx, tt.original.DeepCopy(), client.RawPatch(types.MergePatchType, []byte(tt.externalChanges)))).To(Succeed())
				}
			}

			s := scope.New(&clusterv1.Cluster{})
			if tt.original != nil {
				current := builder.TestInfrastructureCluster("", "").Build()
				g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.original), current)).To(Succeed())
				s.Current.InfrastructureCluster = current
			}
			s.Desired = &scope.ClusterState{InfrastructureCluster: tt.desired.DeepCopy()}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}
			err = r.reconcileInfrastructureCluster(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy() // this is required otherwise Get will modify tt.want
			err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			// Spec
			wantSpec, ok, err := unstructured.NestedMap(tt.want.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			gotSpec, ok, err := unstructured.NestedMap(got.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())
			for k, v := range wantSpec {
				g.Expect(gotSpec).To(HaveKeyWithValue(k, v))
			}

			if tt.desired != nil {
				g.Expect(env.CleanupAndWait(ctx, tt.desired)).To(Succeed())
			}
		})
	}
}

func TestReconcileControlPlane(t *testing.T) {
	g := NewWithT(t)

	// Objects for testing reconciliation of a control plane without machines.

	// Create cluster class which does not require controlPlaneInfrastructure.
	ccWithoutControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{}

	// Create ControlPlaneObject without machine templates.
	controlPlaneWithoutInfrastructure := builder.TestControlPlane(metav1.NamespaceDefault, "cp1").
		WithSpecFields(map[string]interface{}{"spec.foo": "foo"}).
		Build()

	// Create desired ControlPlaneObject without machine templates but introducing some change.
	controlPlaneWithoutInfrastructureWithChanges := controlPlaneWithoutInfrastructure.DeepCopy()
	g.Expect(unstructured.SetNestedField(controlPlaneWithoutInfrastructureWithChanges.UnstructuredContent(), "foo-changed", "spec", "foo")).To(Succeed())

	// Build a patch used to simulate instance specific changes made by an external controller, and build the expected control plane object.
	controlPlaneWithoutInfrastructureExternalChanges := "{ \"spec\": { \"bar\": \"bar\" }}"
	controlPlaneWithoutInfrastructureWithExternalChanges := controlPlaneWithoutInfrastructure.DeepCopy()
	g.Expect(unstructured.SetNestedField(controlPlaneWithoutInfrastructureWithExternalChanges.UnstructuredContent(), "bar", "spec", "bar")).To(Succeed())

	// Build a patch used to simulate an external controller overriding a field managed by the topology controller.
	controlPlaneWithoutInfrastructureWithExternalOverridingChanges := "{ \"spec\": { \"foo\": \"foo-override\" }}"

	// Create a desired ControlPlaneObject without machine templates but introducing incompatible changes.
	controlPlaneWithoutInfrastructureWithIncompatibleChanges := controlPlaneWithoutInfrastructure.DeepCopy()
	controlPlaneWithoutInfrastructureWithIncompatibleChanges.SetName("cp1-changed")

	// Objects for testing reconciliation of a control plane with machines.

	// Create cluster class which does not require controlPlaneInfrastructure.
	infrastructureMachineTemplate := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.foo": "foo"}).
		Build()
	ccWithControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{InfrastructureMachineTemplate: infrastructureMachineTemplate}

	// Create ControlPlaneObject with machine templates.
	controlPlaneWithInfrastructure := builder.TestControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		WithSpecFields(map[string]interface{}{"spec.foo": "foo"}).
		Build()

	// Create desired controlPlaneInfrastructure with some change.
	infrastructureMachineTemplateWithChanges := infrastructureMachineTemplate.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplateWithChanges.UnstructuredContent(), "foo-changed", "spec", "template", "spec", "foo")).To(Succeed())

	// Build a patch used to simulate instance specific changes made by an external controller, and build the expected machine infrastructure object.
	infrastructureMachineTemplateExternalChanges := "{ \"spec\": { \"template\": { \"spec\": { \"bar\": \"bar\" } } }}"
	infrastructureMachineTemplateWithExternalChanges := infrastructureMachineTemplate.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplateWithExternalChanges.UnstructuredContent(), "bar", "spec", "template", "spec", "bar")).To(Succeed())

	// Build a patch used to simulate an external controller overriding a field managed by the topology controller.
	infrastructureMachineTemplateExternalOverridingChanges := "{ \"spec\": { \"template\": { \"spec\": { \"foo\": \"foo-override\" } } }}"

	// Create a desired infrastructure machine template with incompatible changes.
	infrastructureMachineTemplateWithIncompatibleChanges := infrastructureMachineTemplate.DeepCopy()
	gvk := infrastructureMachineTemplateWithIncompatibleChanges.GroupVersionKind()
	gvk.Kind = "KindChanged"
	infrastructureMachineTemplateWithIncompatibleChanges.SetGroupVersionKind(gvk)

	tests := []struct {
		name                                 string
		class                                *scope.ControlPlaneBlueprint
		original                             *scope.ControlPlaneState
		controlPlaneExternalChanges          string
		machineInfrastructureExternalChanges string
		desired                              *scope.ControlPlaneState
		want                                 *scope.ControlPlaneState
		wantRotation                         bool
		wantErr                              bool
	}{
		// Testing reconciliation of a control plane without machines.
		{
			name:     "Should create desired ControlPlane without machine infrastructure if the current does not exist",
			class:    ccWithoutControlPlaneInfrastructure,
			original: nil,
			desired:  &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			want:     &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			wantErr:  false,
		},
		{
			name:     "Should update the ControlPlane without machine infrastructure",
			class:    ccWithoutControlPlaneInfrastructure,
			original: &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			desired:  &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructureWithChanges.DeepCopy()},
			want:     &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructureWithChanges.DeepCopy()},
			wantErr:  false,
		},
		{
			name:                        "Should preserve external changes to ControlPlane without machine infrastructure",
			class:                       ccWithoutControlPlaneInfrastructure,
			original:                    &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			controlPlaneExternalChanges: controlPlaneWithoutInfrastructureExternalChanges,
			desired:                     &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			want:                        &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructureWithExternalChanges.DeepCopy()},
			wantErr:                     false,
		},
		{
			name:                        "Should restore template values if overridden by external controllers into a ControlPlane without machine infrastructure",
			class:                       ccWithoutControlPlaneInfrastructure,
			original:                    &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			controlPlaneExternalChanges: controlPlaneWithoutInfrastructureWithExternalOverridingChanges,
			desired:                     &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			want:                        &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			wantErr:                     false,
		},
		{
			name:     "Fail on updating ControlPlane without machine infrastructure in case of incompatible changes",
			class:    ccWithoutControlPlaneInfrastructure,
			original: &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructure.DeepCopy()},
			desired:  &scope.ControlPlaneState{Object: controlPlaneWithoutInfrastructureWithIncompatibleChanges.DeepCopy()},
			wantErr:  true,
		},

		// Testing reconciliation of a control plane with machines.
		{
			name:     "Should create desired ControlPlane with machine infrastructure if the current does not exist",
			class:    ccWithControlPlaneInfrastructure,
			original: nil,
			desired:  &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:     &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr:  false,
		},
		{
			name:         "Should rotate machine infrastructure in case of changes to the desired template",
			class:        ccWithControlPlaneInfrastructure,
			original:     &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired:      &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplateWithChanges.DeepCopy()},
			want:         &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplateWithChanges.DeepCopy()},
			wantRotation: true,
			wantErr:      false,
		},
		{
			name:                                 "Should preserve external changes to ControlPlane's machine infrastructure", // NOTE: template are not expected to mutate, this is for extra safety.
			class:                                ccWithControlPlaneInfrastructure,
			original:                             &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			machineInfrastructureExternalChanges: infrastructureMachineTemplateExternalChanges,
			desired:                              &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:                                 &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplateWithExternalChanges.DeepCopy()},
			wantRotation:                         false,
			wantErr:                              false,
		},
		{
			name:                                 "Should restore template values if overridden by external controllers into the ControlPlane's machine infrastructure", // NOTE: template are not expected to mutate, this is for extra safety.
			class:                                ccWithControlPlaneInfrastructure,
			original:                             &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			machineInfrastructureExternalChanges: infrastructureMachineTemplateExternalOverridingChanges,
			desired:                              &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:                                 &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantRotation:                         true,
			wantErr:                              false,
		},
		{
			name:     "Fail on updating ControlPlane with a machine infrastructure in case of incompatible changes",
			class:    ccWithControlPlaneInfrastructure,
			original: &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired:  &scope.ControlPlaneState{Object: controlPlaneWithInfrastructure.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplateWithIncompatibleChanges.DeepCopy()},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-control-plane")
			g.Expect(err).ToNot(HaveOccurred())
			if tt.class != nil { // *scope.ControlPlaneBlueprint
				tt.class = prepareControlPlaneBluePrint(tt.class, namespace.GetName())
			}
			if tt.original != nil { // *scope.ControlPlaneState
				tt.original = prepareControlPlaneState(g, tt.original, namespace.GetName())
			}
			if tt.desired != nil { // *scope.ControlPlaneState
				tt.desired = prepareControlPlaneState(g, tt.desired, namespace.GetName())
			}
			if tt.want != nil { // *scope.ControlPlaneState
				tt.want = prepareControlPlaneState(g, tt.want, namespace.GetName())
			}

			s := scope.New(builder.Cluster(namespace.GetName(), "cluster1").Build())
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}
			if tt.class.InfrastructureMachineTemplate != nil {
				s.Blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
					Ref: contract.ObjToRef(tt.class.InfrastructureMachineTemplate),
				}
			}

			s.Current.ControlPlane = &scope.ControlPlaneState{}
			if tt.original != nil {
				if tt.original.InfrastructureMachineTemplate != nil {
					// NOTE: it is required to use server side apply to creat the object in order to ensure consistency with the topology controller behaviour.
					g.Expect(env.PatchAndWait(ctx, tt.original.InfrastructureMachineTemplate.DeepCopy(), client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
					// NOTE: it is required to apply instance specific changes with a "plain" Patch operation to simulate a different manger.
					if tt.machineInfrastructureExternalChanges != "" {
						g.Expect(env.Patch(ctx, tt.original.InfrastructureMachineTemplate.DeepCopy(), client.RawPatch(types.MergePatchType, []byte(tt.machineInfrastructureExternalChanges)))).To(Succeed())
					}

					current := builder.TestInfrastructureMachineTemplate("", "").Build()
					g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.original.InfrastructureMachineTemplate), current)).To(Succeed())
					s.Current.ControlPlane.InfrastructureMachineTemplate = current
				}
				if tt.original.Object != nil {
					// NOTE: it is required to use server side apply to creat the object in order to ensure consistency with the topology controller behaviour.
					g.Expect(env.PatchAndWait(ctx, tt.original.Object.DeepCopy(), client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
					// NOTE: it is required to apply instance specific changes with a "plain" Patch operation to simulate a different manger.
					if tt.controlPlaneExternalChanges != "" {
						g.Expect(env.Patch(ctx, tt.original.Object.DeepCopy(), client.RawPatch(types.MergePatchType, []byte(tt.controlPlaneExternalChanges)))).To(Succeed())
					}

					current := builder.TestControlPlane("", "").Build()
					g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.original.Object), current)).To(Succeed())
					s.Current.ControlPlane.Object = current
				}
			}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}

			s.Desired = &scope.ClusterState{
				ControlPlane: &scope.ControlPlaneState{
					Object:                        tt.desired.Object,
					InfrastructureMachineTemplate: tt.desired.InfrastructureMachineTemplate,
				},
			}

			// Run reconcileControlPlane with the states created in the initial section of the test.
			err = r.reconcileControlPlane(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Create ControlPlane object for fetching data into
			gotControlPlaneObject := builder.TestControlPlane("", "").Build()
			err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.want.Object), gotControlPlaneObject)
			g.Expect(err).ToNot(HaveOccurred())

			// check for template rotation.
			gotRotation := false
			var gotInfrastructureMachineRef *corev1.ObjectReference
			if tt.class.InfrastructureMachineTemplate != nil {
				gotInfrastructureMachineRef, err = contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(gotControlPlaneObject)
				g.Expect(err).ToNot(HaveOccurred())
				if tt.original != nil {
					if tt.original.InfrastructureMachineTemplate != nil && tt.original.InfrastructureMachineTemplate.GetName() != gotInfrastructureMachineRef.Name {
						gotRotation = true
						// if template has been rotated, fixup infrastructureRef in the wantControlPlaneObjectSpec before comparison.
						g.Expect(contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(tt.want.Object, refToUnstructured(gotInfrastructureMachineRef))).To(Succeed())
					}
				}
			}
			g.Expect(gotRotation).To(Equal(tt.wantRotation))

			// Get the spec from the ControlPlaneObject we are expecting
			wantControlPlaneObjectSpec, ok, err := unstructured.NestedMap(tt.want.Object.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			// Get the spec from the ControlPlaneObject we got from the client.Get
			gotControlPlaneObjectSpec, ok, err := unstructured.NestedMap(gotControlPlaneObject.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			for k, v := range wantControlPlaneObjectSpec {
				g.Expect(gotControlPlaneObjectSpec).To(HaveKeyWithValue(k, v))
			}
			for k, v := range tt.want.Object.GetLabels() {
				g.Expect(gotControlPlaneObject.GetLabels()).To(HaveKeyWithValue(k, v))
			}

			// Check the infrastructure template
			if tt.want.InfrastructureMachineTemplate != nil {
				// Check to see if the controlPlaneObject has been updated with a new template.
				// This check is just for the naming format uses by generated templates - here it's templateName-*
				// This check is only performed when we had an initial template that has been changed
				if gotRotation {
					pattern := fmt.Sprintf("%s.*", controlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name))
					ok, err := regexp.Match(pattern, []byte(gotInfrastructureMachineRef.Name))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ok).To(BeTrue())
				}

				// Create object to hold the queried InfrastructureMachineTemplate
				gotInfrastructureMachineTemplateKey := client.ObjectKey{Namespace: gotInfrastructureMachineRef.Namespace, Name: gotInfrastructureMachineRef.Name}
				gotInfrastructureMachineTemplate := builder.TestInfrastructureMachineTemplate("", "").Build()
				err = env.GetAPIReader().Get(ctx, gotInfrastructureMachineTemplateKey, gotInfrastructureMachineTemplate)
				g.Expect(err).ToNot(HaveOccurred())

				// Get the spec from the InfrastructureMachineTemplate we are expecting
				wantInfrastructureMachineTemplateSpec, ok, err := unstructured.NestedMap(tt.want.InfrastructureMachineTemplate.UnstructuredContent(), "spec")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ok).To(BeTrue())

				// Get the spec from the InfrastructureMachineTemplate we got from the client.Get
				gotInfrastructureMachineTemplateSpec, ok, err := unstructured.NestedMap(gotInfrastructureMachineTemplate.UnstructuredContent(), "spec")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ok).To(BeTrue())

				// Compare all keys and values in the InfrastructureMachineTemplate Spec
				for k, v := range wantInfrastructureMachineTemplateSpec {
					g.Expect(gotInfrastructureMachineTemplateSpec).To(HaveKeyWithValue(k, v))
				}

				// Check to see that labels are as expected on the object
				for k, v := range tt.want.InfrastructureMachineTemplate.GetLabels() {
					g.Expect(gotInfrastructureMachineTemplate.GetLabels()).To(HaveKeyWithValue(k, v))
				}

				// If the template was rotated during the reconcile we want to make sure the old template was deleted.
				if gotRotation {
					obj := &unstructured.Unstructured{}
					obj.SetAPIVersion(builder.InfrastructureGroupVersion.String())
					obj.SetKind(builder.GenericInfrastructureMachineTemplateKind)
					err := r.Client.Get(ctx, client.ObjectKey{
						Namespace: tt.original.InfrastructureMachineTemplate.GetNamespace(),
						Name:      tt.original.InfrastructureMachineTemplate.GetName(),
					}, obj)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			}
		})
	}
}

func TestReconcileControlPlaneMachineHealthCheck(t *testing.T) {
	// Create InfrastructureMachineTemplates for test cases
	infrastructureMachineTemplate := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()

	mhcClass := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		},
	}
	maxUnhealthy := intstr.Parse("45%")
	// Create clusterClasses requiring controlPlaneInfrastructure and one not.
	ccWithControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{
		InfrastructureMachineTemplate: infrastructureMachineTemplate,
		MachineHealthCheck:            mhcClass,
	}
	ccWithoutControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{
		MachineHealthCheck: mhcClass,
	}

	// Create ControlPlane Object.
	controlPlane1 := builder.TestControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()

	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "cp1").
		WithSelector(*selectorForControlPlaneMHC()).
		WithUnhealthyConditions(mhcClass.UnhealthyConditions).
		WithClusterName("cluster1")

	tests := []struct {
		name    string
		class   *scope.ControlPlaneBlueprint
		current *scope.ControlPlaneState
		desired *scope.ControlPlaneState
		want    *clusterv1.MachineHealthCheck
	}{
		{
			name:    "Should create desired ControlPlane MachineHealthCheck for a new ControlPlane",
			class:   ccWithControlPlaneInfrastructure,
			current: nil,
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			want: mhcBuilder.DeepCopy().
				WithDefaulter(true).
				Build(),
		},
		{
			name:  "Should not create ControlPlane MachineHealthCheck when no MachineInfrastructure is defined",
			class: ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object: controlPlane1.DeepCopy(),
				// Note this creation would be blocked by the validation Webhook. MHC with no MachineInfrastructure is not allowed.
				MachineHealthCheck: mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object: controlPlane1.DeepCopy(),
				// ControlPlane does not have defined MachineInfrastructure.
				// InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
			},
			want: nil,
		},
		{
			name:  "Should update ControlPlane MachineHealthCheck when changed in desired state",
			class: ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.WithMaxUnhealthy(&maxUnhealthy).Build(),
			},
			// Want to get the updated version of the MachineHealthCheck after reconciliation.
			want: mhcBuilder.DeepCopy().WithMaxUnhealthy(&maxUnhealthy).
				WithDefaulter(true).
				Build(),
		},
		{
			name:  "Should delete ControlPlane MachineHealthCheck when removed from desired state",
			class: ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				// MachineHealthCheck removed from the desired state of the ControlPlane
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-control-plane")
			g.Expect(err).ToNot(HaveOccurred())
			if tt.class != nil {
				tt.class = prepareControlPlaneBluePrint(tt.class, namespace.GetName())
			}
			if tt.current != nil {
				tt.current = prepareControlPlaneState(g, tt.current, namespace.GetName())
			}
			if tt.desired != nil {
				tt.desired = prepareControlPlaneState(g, tt.desired, namespace.GetName())
			}
			if tt.want != nil {
				tt.want.SetNamespace(namespace.GetName())
			}

			s := scope.New(builder.Cluster(namespace.GetName(), "cluster1").Build())
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}
			if tt.class.InfrastructureMachineTemplate != nil {
				s.Blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
					Ref: contract.ObjToRef(tt.class.InfrastructureMachineTemplate),
				}
			}

			s.Current.ControlPlane = &scope.ControlPlaneState{}
			if tt.current != nil {
				s.Current.ControlPlane = tt.current
				if tt.current.Object != nil {
					g.Expect(env.PatchAndWait(ctx, tt.current.Object, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				}
				if tt.current.InfrastructureMachineTemplate != nil {
					g.Expect(env.PatchAndWait(ctx, tt.current.InfrastructureMachineTemplate, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				}
				if tt.current.MachineHealthCheck != nil {
					g.Expect(env.PatchAndWait(ctx, tt.current.MachineHealthCheck, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				}
			}

			// copy over uid of created and desired ControlPlane
			if tt.current != nil && tt.current.Object != nil && tt.desired != nil && tt.desired.Object != nil {
				tt.desired.Object.SetUID(tt.current.Object.GetUID())
			}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}

			s.Desired = &scope.ClusterState{
				ControlPlane: tt.desired,
			}

			// Run reconcileControlPlane with the states created in the initial section of the test.
			err = r.reconcileControlPlane(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			gotCP := s.Desired.ControlPlane.Object.DeepCopy()
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: namespace.GetName(), Name: controlPlane1.GetName()}, gotCP)).To(Succeed())

			// Create MachineHealthCheck object for fetching data into
			gotMHC := &clusterv1.MachineHealthCheck{}
			err = env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: namespace.GetName(), Name: controlPlane1.GetName()}, gotMHC)

			// Nil case: If we want to find nothing (i.e. delete or MHC not created) and the Get call returns a NotFound error from the API the test succeeds.
			if tt.want == nil && apierrors.IsNotFound(err) {
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotMHC).To(EqualObject(tt.want, IgnoreAutogeneratedMetadata, IgnorePaths{{"kind"}, {"apiVersion"}}))
		})
	}
}

func TestReconcileMachineDeployments(t *testing.T) {
	g := NewWithT(t)

	// Write the config file to access the test env for debugging.
	// g.Expect(os.WriteFile("test.conf", kubeconfig.FromEnvTestConfig(env.Config, &clusterv1.Cluster{
	// 	ObjectMeta: metav1.ObjectMeta{Name: "test"},
	// }), 0777)).To(Succeed())

	infrastructureMachineTemplate1 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-1").Build()
	bootstrapTemplate1 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-1").Build()
	md1 := newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate1, bootstrapTemplate1, nil)

	infrastructureMachineTemplate2 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-2").Build()
	bootstrapTemplate2 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-2").Build()
	md2 := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2, bootstrapTemplate2, nil)
	infrastructureMachineTemplate2WithChanges := infrastructureMachineTemplate2.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate2WithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	md2WithRotatedInfrastructureMachineTemplate := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2WithChanges, bootstrapTemplate2, nil)

	infrastructureMachineTemplate3 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-3").Build()
	bootstrapTemplate3 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-3").Build()
	md3 := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3, nil)
	bootstrapTemplate3WithChanges := bootstrapTemplate3.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate3WithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	md3WithRotatedBootstrapTemplate := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges, nil)
	bootstrapTemplate3WithChangeKind := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChangeKind.SetKind("AnotherGenericBootstrapTemplate")
	md3WithRotatedBootstrapTemplateChangedKind := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges, nil)

	infrastructureMachineTemplate4 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-4").Build()
	bootstrapTemplate4 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-4").Build()
	md4 := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4, bootstrapTemplate4, nil)
	infrastructureMachineTemplate4WithChanges := infrastructureMachineTemplate4.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate4WithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	bootstrapTemplate4WithChanges := bootstrapTemplate4.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate4WithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	md4WithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4WithChanges, bootstrapTemplate4WithChanges, nil)

	infrastructureMachineTemplate4m := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-4m").Build()
	bootstrapTemplate4m := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-4m").Build()
	md4m := newFakeMachineDeploymentTopologyState("md-4m", infrastructureMachineTemplate4m, bootstrapTemplate4m, nil)
	infrastructureMachineTemplate4mWithChanges := infrastructureMachineTemplate4m.DeepCopy()
	infrastructureMachineTemplate4mWithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate4mWithChanges := bootstrapTemplate4m.DeepCopy()
	bootstrapTemplate4mWithChanges.SetLabels(map[string]string{"foo": "bar"})
	md4mWithInPlaceUpdatedTemplates := newFakeMachineDeploymentTopologyState("md-4m", infrastructureMachineTemplate4mWithChanges, bootstrapTemplate4mWithChanges, nil)

	infrastructureMachineTemplate5 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-5").Build()
	bootstrapTemplate5 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-5").Build()
	md5 := newFakeMachineDeploymentTopologyState("md-5", infrastructureMachineTemplate5, bootstrapTemplate5, nil)
	infrastructureMachineTemplate5WithChangedKind := infrastructureMachineTemplate5.DeepCopy()
	infrastructureMachineTemplate5WithChangedKind.SetKind("ChangedKind")
	md5WithChangedInfrastructureMachineTemplateKind := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate5WithChangedKind, bootstrapTemplate5, nil)

	infrastructureMachineTemplate6 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-6").Build()
	bootstrapTemplate6 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-6").Build()
	md6 := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6, nil)
	bootstrapTemplate6WithChangedNamespace := bootstrapTemplate6.DeepCopy()
	bootstrapTemplate6WithChangedNamespace.SetNamespace("ChangedNamespace")
	md6WithChangedBootstrapTemplateNamespace := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6WithChangedNamespace, nil)

	infrastructureMachineTemplate7 := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-7").Build()
	bootstrapTemplate7 := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-7").Build()
	md7 := newFakeMachineDeploymentTopologyState("md-7", infrastructureMachineTemplate7, bootstrapTemplate7, nil)

	infrastructureMachineTemplate8Create := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-create").Build()
	bootstrapTemplate8Create := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-create").Build()
	md8Create := newFakeMachineDeploymentTopologyState("md-8-create", infrastructureMachineTemplate8Create, bootstrapTemplate8Create, nil)
	infrastructureMachineTemplate8Delete := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-delete").Build()
	bootstrapTemplate8Delete := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-delete").Build()
	md8Delete := newFakeMachineDeploymentTopologyState("md-8-delete", infrastructureMachineTemplate8Delete, bootstrapTemplate8Delete, nil)
	infrastructureMachineTemplate8Update := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-update").Build()
	bootstrapTemplate8Update := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-update").Build()
	md8Update := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8Update, bootstrapTemplate8Update, nil)
	infrastructureMachineTemplate8UpdateWithChanges := infrastructureMachineTemplate8Update.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate8UpdateWithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	bootstrapTemplate8UpdateWithChanges := bootstrapTemplate8Update.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate8UpdateWithChanges.Object, "foo", "spec", "template", "spec", "foo")).To(Succeed())
	md8UpdateWithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8UpdateWithChanges, bootstrapTemplate8UpdateWithChanges, nil)

	infrastructureMachineTemplate9m := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-9m").Build()
	bootstrapTemplate9m := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-9m").Build()
	md9 := newFakeMachineDeploymentTopologyState("md-9m", infrastructureMachineTemplate9m, bootstrapTemplate9m, nil)
	md9.Object.Spec.Template.ObjectMeta.Labels = map[string]string{clusterv1.ClusterLabelName: "cluster-1", "foo": "bar"}
	md9.Object.Spec.Selector.MatchLabels = map[string]string{clusterv1.ClusterLabelName: "cluster-1", "foo": "bar"}
	md9WithInstanceSpecificTemplateMetadataAndSelector := newFakeMachineDeploymentTopologyState("md-9m", infrastructureMachineTemplate9m, bootstrapTemplate9m, nil)
	md9WithInstanceSpecificTemplateMetadataAndSelector.Object.Spec.Template.ObjectMeta.Labels = map[string]string{"foo": "bar"}
	md9WithInstanceSpecificTemplateMetadataAndSelector.Object.Spec.Selector.MatchLabels = map[string]string{"foo": "bar"}

	tests := []struct {
		name                                      string
		current                                   []*scope.MachineDeploymentState
		desired                                   []*scope.MachineDeploymentState
		want                                      []*scope.MachineDeploymentState
		wantInfrastructureMachineTemplateRotation map[string]bool
		wantBootstrapTemplateRotation             map[string]bool
		wantErr                                   bool
	}{
		{
			name:    "Should create desired MachineDeployment if the current does not exists yet",
			current: nil,
			desired: []*scope.MachineDeploymentState{md1},
			want:    []*scope.MachineDeploymentState{md1},
			wantErr: false,
		},
		{
			name:    "No-op if current MachineDeployment is equal to desired",
			current: []*scope.MachineDeploymentState{md1},
			desired: []*scope.MachineDeploymentState{md1},
			want:    []*scope.MachineDeploymentState{md1},
			wantErr: false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate rotation",
			current: []*scope.MachineDeploymentState{md2},
			desired: []*scope.MachineDeploymentState{md2WithRotatedInfrastructureMachineTemplate},
			want:    []*scope.MachineDeploymentState{md2WithRotatedInfrastructureMachineTemplate},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-2": true},
			wantErr: false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation",
			current:                       []*scope.MachineDeploymentState{md3},
			desired:                       []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplate},
			want:                          []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplate},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation with changed kind",
			current:                       []*scope.MachineDeploymentState{md3},
			desired:                       []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplateChangedKind},
			want:                          []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplateChangedKind},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate and BootstrapTemplate rotation",
			current: []*scope.MachineDeploymentState{md4},
			desired: []*scope.MachineDeploymentState{md4WithRotatedTemplates},
			want:    []*scope.MachineDeploymentState{md4WithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-4": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-4": true},
			wantErr:                                   false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate and BootstrapTemplate without rotation",
			current: []*scope.MachineDeploymentState{md4m},
			desired: []*scope.MachineDeploymentState{md4mWithInPlaceUpdatedTemplates},
			want:    []*scope.MachineDeploymentState{md4mWithInPlaceUpdatedTemplates},
			wantErr: false,
		},
		{
			name:    "Should fail update MachineDeployment because of changed InfrastructureMachineTemplate kind",
			current: []*scope.MachineDeploymentState{md5},
			desired: []*scope.MachineDeploymentState{md5WithChangedInfrastructureMachineTemplateKind},
			wantErr: true,
		},
		{
			name:    "Should fail update MachineDeployment because of changed BootstrapTemplate namespace",
			current: []*scope.MachineDeploymentState{md6},
			desired: []*scope.MachineDeploymentState{md6WithChangedBootstrapTemplateNamespace},
			wantErr: true,
		},
		{
			name:    "Should delete MachineDeployment",
			current: []*scope.MachineDeploymentState{md7},
			desired: []*scope.MachineDeploymentState{},
			want:    []*scope.MachineDeploymentState{},
			wantErr: false,
		},
		{
			name:    "Should create, update and delete MachineDeployments",
			current: []*scope.MachineDeploymentState{md8Update, md8Delete},
			desired: []*scope.MachineDeploymentState{md8Create, md8UpdateWithRotatedTemplates},
			want:    []*scope.MachineDeploymentState{md8Create, md8UpdateWithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-8-update": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-8-update": true},
			wantErr:                                   false,
		},
		{
			name:    "Enforce template metadata",
			current: []*scope.MachineDeploymentState{md9WithInstanceSpecificTemplateMetadataAndSelector},
			desired: []*scope.MachineDeploymentState{md9},
			want:    []*scope.MachineDeploymentState{md9},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-machine-deployments")
			g.Expect(err).ToNot(HaveOccurred())
			for i, s := range tt.current {
				tt.current[i] = prepareMachineDeploymentState(s, namespace.GetName())
			}
			for i, s := range tt.desired {
				tt.desired[i] = prepareMachineDeploymentState(s, namespace.GetName())
			}
			for i, s := range tt.want {
				tt.want[i] = prepareMachineDeploymentState(s, namespace.GetName())
			}

			for _, s := range tt.current {
				g.Expect(env.PatchAndWait(ctx, s.InfrastructureMachineTemplate, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				g.Expect(env.PatchAndWait(ctx, s.BootstrapTemplate, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				g.Expect(env.PatchAndWait(ctx, s.Object, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
			}

			currentMachineDeploymentStates := toMachineDeploymentTopologyStateMap(tt.current)
			s := scope.New(builder.Cluster(metav1.NamespaceDefault, "cluster-1").Build())
			s.Current.MachineDeployments = currentMachineDeploymentStates

			s.Desired = &scope.ClusterState{MachineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}
			err = r.reconcileMachineDeployments(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var gotMachineDeploymentList clusterv1.MachineDeploymentList
			g.Expect(env.GetAPIReader().List(ctx, &gotMachineDeploymentList, &client.ListOptions{Namespace: namespace.GetName()})).To(Succeed())
			g.Expect(gotMachineDeploymentList.Items).To(HaveLen(len(tt.want)))

			for _, wantMachineDeploymentState := range tt.want {
				for _, gotMachineDeployment := range gotMachineDeploymentList.Items {
					if wantMachineDeploymentState.Object.Name != gotMachineDeployment.Name {
						continue
					}
					currentMachineDeploymentTopologyName := wantMachineDeploymentState.Object.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
					currentMachineDeploymentState := currentMachineDeploymentStates[currentMachineDeploymentTopologyName]

					// Copy over the name of the newly created InfrastructureRef and Bootsrap.ConfigRef because they get a generated name
					wantMachineDeploymentState.Object.Spec.Template.Spec.InfrastructureRef.Name = gotMachineDeployment.Spec.Template.Spec.InfrastructureRef.Name
					if gotMachineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
						wantMachineDeploymentState.Object.Spec.Template.Spec.Bootstrap.ConfigRef.Name = gotMachineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef.Name
					}

					// Compare MachineDeployment.
					// Note: We're intentionally only comparing Spec as otherwise we would have to account for
					// empty vs. filled out TypeMeta.
					g.Expect(gotMachineDeployment.Spec).To(Equal(wantMachineDeploymentState.Object.Spec))

					// Compare BootstrapTemplate.
					gotBootstrapTemplateRef := gotMachineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef
					gotBootstrapTemplate := unstructured.Unstructured{}
					gotBootstrapTemplate.SetKind(gotBootstrapTemplateRef.Kind)
					gotBootstrapTemplate.SetAPIVersion(gotBootstrapTemplateRef.APIVersion)

					err = env.GetAPIReader().Get(ctx, client.ObjectKey{
						Namespace: gotBootstrapTemplateRef.Namespace,
						Name:      gotBootstrapTemplateRef.Name,
					}, &gotBootstrapTemplate)

					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(&gotBootstrapTemplate).To(EqualObject(wantMachineDeploymentState.BootstrapTemplate, IgnoreAutogeneratedMetadata, IgnoreNameGenerated))

					// Check BootstrapTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentState != nil && currentMachineDeploymentState.BootstrapTemplate != nil {
						if tt.wantBootstrapTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentState.BootstrapTemplate.GetName()).ToNot(Equal(gotBootstrapTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentState.BootstrapTemplate.GetName()).To(Equal(gotBootstrapTemplate.GetName()))
						}
					}

					// Compare InfrastructureMachineTemplate.
					gotInfrastructureMachineTemplateRef := gotMachineDeployment.Spec.Template.Spec.InfrastructureRef
					gotInfrastructureMachineTemplate := unstructured.Unstructured{}
					gotInfrastructureMachineTemplate.SetKind(gotInfrastructureMachineTemplateRef.Kind)
					gotInfrastructureMachineTemplate.SetAPIVersion(gotInfrastructureMachineTemplateRef.APIVersion)

					err = env.GetAPIReader().Get(ctx, client.ObjectKey{
						Namespace: gotInfrastructureMachineTemplateRef.Namespace,
						Name:      gotInfrastructureMachineTemplateRef.Name,
					}, &gotInfrastructureMachineTemplate)

					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(&gotInfrastructureMachineTemplate).To(EqualObject(wantMachineDeploymentState.InfrastructureMachineTemplate, IgnoreAutogeneratedMetadata, IgnoreNameGenerated))

					// Check InfrastructureMachineTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentState != nil && currentMachineDeploymentState.InfrastructureMachineTemplate != nil {
						if tt.wantInfrastructureMachineTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentState.InfrastructureMachineTemplate.GetName()).ToNot(Equal(gotInfrastructureMachineTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentState.InfrastructureMachineTemplate.GetName()).To(Equal(gotInfrastructureMachineTemplate.GetName()))
						}
					}
				}
			}
		})
	}
}

// TestReconcileReferencedObjectSequences tests multiple subsequent calls to reconcileReferencedObject
// for a control-plane object to verify that the objects are reconciled as expected by tracking managed fields correctly.
// NOTE: by Extension this tests validates managed field handling in mergePatches, and thus its usage in other parts of the
// codebase.
func TestReconcileReferencedObjectSequences(t *testing.T) {
	// g := NewWithT(t)
	// Write the config file to access the test env for debugging.
	// g.Expect(os.WriteFile("test.conf", kubeconfig.FromEnvTestConfig(env.Config, &clusterv1.Cluster{
	// 	ObjectMeta: metav1.ObjectMeta{Name: "test"},
	// }), 0777)).To(Succeed())

	type object struct {
		spec map[string]interface{}
	}

	type externalStep struct {
		name string
		// object is the state of the control-plane object after an external modification.
		object object
	}
	type reconcileStep struct {
		name string
		// desired is the desired control-plane object handed over to reconcileReferencedObject.
		desired object
		// want is the expected control-plane object after calling reconcileReferencedObject.
		want object
	}

	tests := []struct {
		name           string
		reconcileSteps []interface{}
	}{
		{
			name: "Should drop nested field",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// and most specifically it verifies that when a field in a template is deleted, it gets deleted
			// from the generated object (and it is not being treated as instance specific value).
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Drop enable-hostpath-provisioner",
					desired: object{
						spec: nil,
					},
					want: object{
						spec: nil,
					},
				},
			},
		},
		{
			name: "Should drop label",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// and most specifically it verifies that when a template label is deleted, it gets deleted
			// from the generated object (and it is not being treated as instance specific value).
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Drop the label with dots",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// label.with.dots/owned has been removed by e.g a change in Cluster.Topology.ControlPlane.Labels.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// Reconcile to drop label.with.dots/owned label.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should enforce field",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// by reverting user changes to a manged field.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile",
					desired: object{
						spec: map[string]interface{}{
							"foo": "ccValue",
						},
					},
					want: object{
						spec: map[string]interface{}{
							"foo": "ccValue",
						},
					},
				},
				externalStep{
					name: "User changes value",
					object: object{
						spec: map[string]interface{}{
							"foo": "userValue",
						},
					},
				},
				reconcileStep{
					name: "Reconcile overwrites value",
					desired: object{
						spec: map[string]interface{}{
							// ClusterClass still proposing the old value.
							"foo": "ccValue",
						},
					},
					want: object{
						spec: map[string]interface{}{
							// Reconcile to restore the old value.
							"foo": "ccValue",
						},
					},
				},
			},
		},
		{
			name: "Should preserve user-defined field while dropping managed field",
			// Note: This test verifies that Topology treats changes to fields existing in templates as authoritative
			// but allows setting additional/instance-specific values.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
				},
				externalStep{
					name: "User adds an additional extraArg",
					object: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											// User adds enable-garbage-collector.
											"enable-garbage-collector": "true",
										},
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Previously set extraArgs is dropped from KCP, user-specified field is preserved.",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											// Reconcile to drop enable-hostpath-provisioner,
											// while preserving user-defined enable-garbage-collector field.
											"enable-garbage-collector": "true",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should preserve user-defined object field while dropping managed fields",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// but allows setting additional/instance-specific values.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{},
						},
					},
				},
				externalStep{
					name: "User adds an additional object",
					object: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"infrastructureRef": map[string]interface{}{
									"apiVersion": "foo/v1alpha1",
									"kind":       "Foo",
								},
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass starts having an opinion about some fields",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"foo": "foo",
									},
								},
								"nodeDeletionTimeout": "10m",
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								// User fields are preserved.
								"infrastructureRef": map[string]interface{}{
									"apiVersion": "foo/v1alpha1",
									"kind":       "Foo",
								},
								// ClusterClass authoritative fields are added.
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"foo": "foo",
									},
								},
								"nodeDeletionTimeout": "10m",
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass stops having an opinion on the field",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"foo": "foo",
									},
								},
								// clusterClassField has been removed by e.g a change in ClusterClass (and extraArgs with it).
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								// Reconcile to drop clusterClassField,
								// while preserving user-defined field and clusterClassField.
								"infrastructureRef": map[string]interface{}{
									"apiVersion": "foo/v1alpha1",
									"kind":       "Foo",
								},
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"foo": "foo",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass stops having an opinion on the object",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								// clusterClassObject has been removed by e.g a change in ClusterClass (and extraArgs with it).
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								// Reconcile to drop clusterClassObject,
								// while preserving user-defined field.
								"infrastructureRef": map[string]interface{}{
									"apiVersion": "foo/v1alpha1",
									"kind":       "Foo",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-ref-obj-seq")
			g.Expect(err).ToNot(HaveOccurred())

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}

			s := scope.New(&clusterv1.Cluster{})
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}

			for i, step := range tt.reconcileSteps {
				var currentControlPlane *unstructured.Unstructured

				// Get current ControlPlane (on later steps).
				if i > 0 {
					currentControlPlane = &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       builder.TestControlPlaneKind,
							"apiVersion": builder.ControlPlaneGroupVersion.String(),
						},
					}
					g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: namespace.GetName(), Name: "my-cluster"}, currentControlPlane)).To(Succeed())
				}

				if step, ok := step.(externalStep); ok {
					// This is a user step, so let's just update the object using SSA.
					obj := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       builder.TestControlPlaneKind,
							"apiVersion": builder.ControlPlaneGroupVersion.String(),
							"metadata": map[string]interface{}{
								"name":      "my-cluster",
								"namespace": namespace.GetName(),
							},
							"spec": step.object.spec,
						},
					}
					err := env.PatchAndWait(ctx, obj, client.FieldOwner("other-controller"), client.ForceOwnership)
					g.Expect(err).ToNot(HaveOccurred())
					continue
				}

				if step, ok := step.(reconcileStep); ok {
					// This is a reconcile step, so let's execute a reconcile and then validate the result.

					// Set the current control plane.
					s.Current.ControlPlane = &scope.ControlPlaneState{
						Object: currentControlPlane,
					}
					// Set the desired control plane.
					s.Desired = &scope.ClusterState{
						ControlPlane: &scope.ControlPlaneState{
							Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"kind":       builder.TestControlPlaneKind,
									"apiVersion": builder.ControlPlaneGroupVersion.String(),
									"metadata": map[string]interface{}{
										"name":      "my-cluster",
										"namespace": namespace.GetName(),
									},
								},
							},
						},
					}
					if step.desired.spec != nil {
						s.Desired.ControlPlane.Object.Object["spec"] = step.desired.spec
					}

					// Execute a reconcile.0
					g.Expect(r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
						cluster: s.Current.Cluster,
						current: s.Current.ControlPlane.Object,
						desired: s.Desired.ControlPlane.Object,
					})).To(Succeed())

					// Build the object for comparison.
					want := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       builder.TestControlPlaneKind,
							"apiVersion": builder.ControlPlaneGroupVersion.String(),
							"metadata": map[string]interface{}{
								"name":      "my-cluster",
								"namespace": namespace.GetName(),
							},
						},
					}
					if step.want.spec != nil {
						want.Object["spec"] = step.want.spec
					}

					// Get the reconciled object.
					got := want.DeepCopy() // this is required otherwise Get will modify want
					g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: namespace.GetName(), Name: "my-cluster"}, got)).To(Succeed())

					// Compare want with got.
					// Ignore .metadata.resourceVersion and .metadata.annotations as we don't care about them in this test.
					unstructured.RemoveNestedField(got.Object, "metadata", "resourceVersion")
					unstructured.RemoveNestedField(got.Object, "metadata", "annotations")
					unstructured.RemoveNestedField(got.Object, "metadata", "creationTimestamp")
					unstructured.RemoveNestedField(got.Object, "metadata", "generation")
					unstructured.RemoveNestedField(got.Object, "metadata", "managedFields")
					unstructured.RemoveNestedField(got.Object, "metadata", "uid")
					unstructured.RemoveNestedField(got.Object, "metadata", "selfLink")
					g.Expect(got).To(EqualObject(want), fmt.Sprintf("Step %q failed: %v", step.name, cmp.Diff(want, got)))
					continue
				}

				panic(fmt.Errorf("unknown step type %T", step))
			}
		})
	}
}

func TestReconcileMachineDeploymentMachineHealthCheck(t *testing.T) {
	md := builder.MachineDeployment(metav1.NamespaceDefault, "md-1").WithLabels(
		map[string]string{
			clusterv1.ClusterTopologyMachineDeploymentLabelName: "machine-deployment-one",
		}).
		Build()

	maxUnhealthy := intstr.Parse("45%")
	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "md-1").
		WithSelector(*selectorForMachineDeploymentMHC(md)).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithClusterName("cluster1")

	infrastructureMachineTemplate := builder.TestInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-1").Build()
	bootstrapTemplate := builder.TestBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-1").Build()

	tests := []struct {
		name    string
		current []*scope.MachineDeploymentState
		desired []*scope.MachineDeploymentState
		want    []*clusterv1.MachineHealthCheck
	}{
		{
			name:    "Create a MachineHealthCheck if the MachineDeployment is being created",
			current: nil,
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().WithDefaulter(true).Build()},
		},
		{
			name: "Create a new MachineHealthCheck if the MachineDeployment is modified to include one",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					nil)},
			// MHC is added in the desired state of the MachineDeployment
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().WithDefaulter(true).Build()}},
		{
			name: "Update MachineHealthCheck spec adding a field if the spec adds a field",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().WithMaxUnhealthy(&maxUnhealthy).Build())},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().
					WithMaxUnhealthy(&maxUnhealthy).
					WithDefaulter(true).
					Build()},
		},
		{
			name: "Update MachineHealthCheck spec removing a field if the spec removes a field",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().WithMaxUnhealthy(&maxUnhealthy).Build()),
			},
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().WithDefaulter(true).Build(),
			},
		},
		{
			name: "Delete MachineHealthCheck spec if the MachineDeployment is modified to remove an existing one",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate, nil)},
			want:    []*clusterv1.MachineHealthCheck{},
		},
		{
			name: "Delete MachineHealthCheck spec if the MachineDeployment is deleted",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{},
			want:    []*clusterv1.MachineHealthCheck{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create namespace and modify input to have correct namespace set
			namespace, err := env.CreateNamespace(ctx, "reconcile-md-mhc")
			g.Expect(err).ToNot(HaveOccurred())
			for i, s := range tt.current {
				tt.current[i] = prepareMachineDeploymentState(s, namespace.GetName())
			}
			for i, s := range tt.desired {
				tt.desired[i] = prepareMachineDeploymentState(s, namespace.GetName())
			}
			for i, mhc := range tt.want {
				tt.want[i] = mhc.DeepCopy()
				tt.want[i].SetNamespace(namespace.GetName())
			}

			uidsByName := map[string]types.UID{}

			for _, mdts := range tt.current {
				g.Expect(env.PatchAndWait(ctx, mdts.Object, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				g.Expect(env.PatchAndWait(ctx, mdts.InfrastructureMachineTemplate, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				g.Expect(env.PatchAndWait(ctx, mdts.BootstrapTemplate, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())

				uidsByName[mdts.Object.Name] = mdts.Object.GetUID()

				if mdts.MachineHealthCheck != nil {
					for i, ref := range mdts.MachineHealthCheck.OwnerReferences {
						ref.UID = mdts.Object.GetUID()
						mdts.MachineHealthCheck.OwnerReferences[i] = ref
					}
					g.Expect(env.PatchAndWait(ctx, mdts.MachineHealthCheck, client.ForceOwnership, client.FieldOwner(structuredmerge.TopologyManagerName))).To(Succeed())
				}
			}

			// copy over ownerReference for desired MachineHealthCheck
			for _, mdts := range tt.desired {
				if mdts.MachineHealthCheck != nil {
					for i, ref := range mdts.MachineHealthCheck.OwnerReferences {
						if uid, ok := uidsByName[ref.Name]; ok {
							ref.UID = uid
							mdts.MachineHealthCheck.OwnerReferences[i] = ref
						}
					}
				}
			}

			currentMachineDeploymentStates := toMachineDeploymentTopologyStateMap(tt.current)
			s := scope.New(builder.Cluster(namespace.GetName(), "cluster-1").Build())
			s.Current.MachineDeployments = currentMachineDeploymentStates

			s.Desired = &scope.ClusterState{MachineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}

			err = r.reconcileMachineDeployments(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			var gotMachineHealthCheckList clusterv1.MachineHealthCheckList
			g.Expect(env.GetAPIReader().List(ctx, &gotMachineHealthCheckList, &client.ListOptions{Namespace: namespace.GetName()})).To(Succeed())
			g.Expect(gotMachineHealthCheckList.Items).To(HaveLen(len(tt.want)))

			g.Expect(len(tt.want)).To(Equal(len(gotMachineHealthCheckList.Items)))

			for _, wantMHC := range tt.want {
				for _, gotMHC := range gotMachineHealthCheckList.Items {
					if wantMHC.Name == gotMHC.Name {
						actual := gotMHC
						// unset UID because it got generated
						for i, ref := range actual.OwnerReferences {
							ref.UID = ""
							actual.OwnerReferences[i] = ref
						}
						g.Expect(wantMHC).To(EqualObject(&actual, IgnoreAutogeneratedMetadata))
					}
				}
			}
		})
	}
}

func newFakeMachineDeploymentTopologyState(name string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured, machineHealthCheck *clusterv1.MachineHealthCheck) *scope.MachineDeploymentState {
	return &scope.MachineDeploymentState{
		Object: builder.MachineDeployment(metav1.NamespaceDefault, name).
			WithInfrastructureTemplate(infrastructureMachineTemplate).
			WithBootstrapTemplate(bootstrapTemplate).
			WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentLabelName: name + "-topology"}).
			WithClusterName("cluster-1").
			WithReplicas(1).
			WithDefaulter(true).
			Build(),
		InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
		BootstrapTemplate:             bootstrapTemplate.DeepCopy(),
		MachineHealthCheck:            machineHealthCheck.DeepCopy(),
	}
}

func toMachineDeploymentTopologyStateMap(states []*scope.MachineDeploymentState) map[string]*scope.MachineDeploymentState {
	ret := map[string]*scope.MachineDeploymentState{}
	for _, state := range states {
		ret[state.Object.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]] = state
	}
	return ret
}

func TestReconciler_reconcileMachineHealthCheck(t *testing.T) {
	// create a controlPlane object with enough information to be used as an OwnerReference for the MachineHealthCheck.
	cp := builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()
	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "cp1").
		WithSelector(*selectorForControlPlaneMHC()).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithClusterName("cluster1")
	tests := []struct {
		name    string
		current *clusterv1.MachineHealthCheck
		desired *clusterv1.MachineHealthCheck
		want    *clusterv1.MachineHealthCheck
		wantErr bool
	}{
		{
			name:    "Create a MachineHealthCheck",
			current: nil,
			desired: mhcBuilder.DeepCopy().Build(),
			want:    mhcBuilder.DeepCopy().WithDefaulter(true).Build(),
		},
		{
			name:    "Update a MachineHealthCheck with changes",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: mhcBuilder.DeepCopy().WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 1000 * time.Minute},
				},
			}).Build(),
			want: mhcBuilder.DeepCopy().WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 1000 * time.Minute},
				},
			}).WithDefaulter(true).Build(),
		},
		{
			name:    "Don't change a MachineHealthCheck with no difference between desired and current",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: mhcBuilder.DeepCopy().Build(),
			want:    mhcBuilder.DeepCopy().WithDefaulter(true).Build(),
		},
		{
			name:    "Delete a MachineHealthCheck",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: nil,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := &clusterv1.MachineHealthCheck{}

			// Create namespace
			namespace, err := env.CreateNamespace(ctx, "reconcile-mhc")
			// Create control plane
			g.Expect(err).ToNot(HaveOccurred())
			localCP := cp.DeepCopy()
			localCP.SetNamespace(namespace.GetName())
			g.Expect(env.CreateAndWait(ctx, localCP)).To(Succeed())
			// Modify test input and re-use control plane uid if necessary
			if tt.current != nil {
				tt.current = tt.current.DeepCopy()
				tt.current.SetNamespace(namespace.GetName())
			}
			if tt.desired != nil {
				tt.desired = tt.desired.DeepCopy()
				tt.desired.SetNamespace(namespace.GetName())
			}
			if tt.want != nil {
				tt.want = tt.want.DeepCopy()
				tt.want.SetNamespace(namespace.GetName())
				if len(tt.want.OwnerReferences) == 1 {
					tt.want.OwnerReferences[0].UID = localCP.GetUID()
				}
			}

			r := Reconciler{
				Client:             env,
				patchHelperFactory: serverSideApplyPatchHelperFactory(env),
				recorder:           env.GetEventRecorderFor("test"),
			}
			if tt.current != nil {
				g.Expect(env.CreateAndWait(ctx, tt.current)).To(Succeed())
			}
			if err := r.reconcileMachineHealthCheck(ctx, tt.current, tt.desired); err != nil {
				if !tt.wantErr {
					t.Errorf("reconcileMachineHealthCheck() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			key := mhcBuilder.Build()
			key.SetNamespace(namespace.GetName())
			if err := env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(key), got); err != nil {
				if !tt.wantErr {
					t.Errorf("reconcileMachineHealthCheck() error = %v, wantErr %v", err, tt.wantErr)
				}
				if apierrors.IsNotFound(err) {
					got = nil
				}
			}

			g.Expect(got).To(EqualObject(tt.want, IgnoreAutogeneratedMetadata, IgnorePaths{{"kind"}, {"apiVersion"}}))
		})
	}
}

// prepareControlPlaneBluePrint deep-copies and returns the input scope and sets
// the given namespace to all relevant objects.
func prepareControlPlaneBluePrint(in *scope.ControlPlaneBlueprint, namespace string) *scope.ControlPlaneBlueprint {
	s := &scope.ControlPlaneBlueprint{}
	if in.InfrastructureMachineTemplate != nil {
		s.InfrastructureMachineTemplate = in.InfrastructureMachineTemplate.DeepCopy()
		if s.InfrastructureMachineTemplate.GetNamespace() == metav1.NamespaceDefault {
			s.InfrastructureMachineTemplate.SetNamespace(namespace)
		}
	}
	if in.MachineHealthCheck != nil {
		s.MachineHealthCheck = in.MachineHealthCheck.DeepCopy()
	}
	if in.Template != nil {
		s.Template = in.Template.DeepCopy()
		if s.Template.GetNamespace() == metav1.NamespaceDefault {
			s.Template.SetNamespace(namespace)
		}
	}
	return s
}

// prepareControlPlaneState deep-copies and returns the input scope and sets
// the given namespace to all relevant objects.
func prepareControlPlaneState(g *WithT, in *scope.ControlPlaneState, namespace string) *scope.ControlPlaneState {
	s := &scope.ControlPlaneState{}
	if in.InfrastructureMachineTemplate != nil {
		s.InfrastructureMachineTemplate = in.InfrastructureMachineTemplate.DeepCopy()
		if s.InfrastructureMachineTemplate.GetNamespace() == metav1.NamespaceDefault {
			s.InfrastructureMachineTemplate.SetNamespace(namespace)
		}
	}
	if in.MachineHealthCheck != nil {
		s.MachineHealthCheck = in.MachineHealthCheck.DeepCopy()
		if s.MachineHealthCheck.GetNamespace() == metav1.NamespaceDefault {
			s.MachineHealthCheck.SetNamespace(namespace)
		}
	}
	if in.Object != nil {
		s.Object = in.Object.DeepCopy()
		if s.Object.GetNamespace() == metav1.NamespaceDefault {
			s.Object.SetNamespace(namespace)
		}
		if current, ok, err := unstructured.NestedString(s.Object.Object, "spec", "machineTemplate", "infrastructureRef", "namespace"); ok && err == nil && current == metav1.NamespaceDefault {
			g.Expect(unstructured.SetNestedField(s.Object.Object, namespace, "spec", "machineTemplate", "infrastructureRef", "namespace")).To(Succeed())
		}
	}
	return s
}

// prepareMachineDeploymentState deep-copies and returns the input scope and sets
// the given namespace to all relevant objects.
func prepareMachineDeploymentState(in *scope.MachineDeploymentState, namespace string) *scope.MachineDeploymentState {
	s := &scope.MachineDeploymentState{}
	if in.BootstrapTemplate != nil {
		s.BootstrapTemplate = in.BootstrapTemplate.DeepCopy()
		if s.BootstrapTemplate.GetNamespace() == metav1.NamespaceDefault {
			s.BootstrapTemplate.SetNamespace(namespace)
		}
	}
	if in.InfrastructureMachineTemplate != nil {
		s.InfrastructureMachineTemplate = in.InfrastructureMachineTemplate.DeepCopy()
		if s.InfrastructureMachineTemplate.GetNamespace() == metav1.NamespaceDefault {
			s.InfrastructureMachineTemplate.SetNamespace(namespace)
		}
	}
	if in.MachineHealthCheck != nil {
		s.MachineHealthCheck = in.MachineHealthCheck.DeepCopy()
		if s.MachineHealthCheck.GetNamespace() == metav1.NamespaceDefault {
			s.MachineHealthCheck.SetNamespace(namespace)
		}
	}
	if in.Object != nil {
		s.Object = in.Object.DeepCopy()
		if s.Object.GetNamespace() == metav1.NamespaceDefault {
			s.Object.SetNamespace(namespace)
		}
		if s.Object.Spec.Template.Spec.Bootstrap.ConfigRef != nil && s.Object.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace == metav1.NamespaceDefault {
			s.Object.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = namespace
		}
		if s.Object.Spec.Template.Spec.InfrastructureRef.Namespace == metav1.NamespaceDefault {
			s.Object.Spec.Template.Spec.InfrastructureRef.Namespace = namespace
		}
	}
	return s
}

// prepareCluster deep-copies and returns the input Cluster and sets
// the given namespace to all relevant objects.
func prepareCluster(in *clusterv1.Cluster, namespace string) *clusterv1.Cluster {
	c := in.DeepCopy()
	if c.Namespace == metav1.NamespaceDefault {
		c.SetNamespace(namespace)
	}
	if c.Spec.InfrastructureRef != nil && c.Spec.InfrastructureRef.Namespace == metav1.NamespaceDefault {
		c.Spec.InfrastructureRef.Namespace = namespace
	}
	if c.Spec.ControlPlaneRef != nil && c.Spec.ControlPlaneRef.Namespace == metav1.NamespaceDefault {
		c.Spec.ControlPlaneRef.Namespace = namespace
	}
	return c
}

func Test_createErrorWithoutObjectName(t *testing.T) {
	detailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
			Message: "DockerMachineTemplate.infrastructure.cluster.x-k8s.io \"docker-template-one\" is invalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
				Causes: []metav1.StatusCause{
					{
						Type:    "FieldValueInvalid",
						Message: "Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
						Field:   "spec.template.spec.preLoadImages",
					},
				},
			},
		},
	}
	expectedDetailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			// The only difference between the two objects should be in the Message section.
			Message: "failed to create DockerMachineTemplate.infrastructure.cluster.x-k8s.io: FieldValueInvalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
				Causes: []metav1.StatusCause{
					{
						Type:    "FieldValueInvalid",
						Message: "Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
						Field:   "spec.template.spec.preLoadImages",
					},
				},
			},
		},
	}
	NoCausesDetailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
			Message: "DockerMachineTemplate.infrastructure.cluster.x-k8s.io \"docker-template-one\" is invalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
			},
		},
	}
	expectedNoCausesDetailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			// The only difference between the two objects should be in the Message section.
			Message: "failed to create DockerMachineTemplate.infrastructure.cluster.x-k8s.io",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
			},
		},
	}
	noDetailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
			Message: "DockerMachineTemplate.infrastructure.cluster.x-k8s.io \"docker-template-one\" is invalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
		},
	}
	expectedNoDetailsError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			// The only difference between the two objects should be in the Message section.
			Message: "failed to create TestControlPlane.controlplane.cluster.x-k8s.io",
		},
	}
	expectedObjectNilError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			// The only difference between the two objects should be in the Message section.
			Message: "failed to create object",
		},
	}
	nonStatusError := errors.New("an unexpected error with unknown information inside")
	expectedNonStatusError := errors.New("failed to create TestControlPlane.controlplane.cluster.x-k8s.io")
	expectedNilObjectNonStatusError := errors.New("failed to create object")
	tests := []struct {
		name     string
		input    error
		expected error
		obj      client.Object
	}{
		{
			name:     "Remove name from status error with details",
			input:    detailsError,
			expected: expectedDetailsError,
			obj:      builder.TestControlPlane("default", "cp1").Build(),
		},
		{
			name:     "Remove name from status error with details but no causes",
			input:    NoCausesDetailsError,
			expected: expectedNoCausesDetailsError,
			obj:      builder.TestControlPlane("default", "cp1").Build(),
		},
		{
			name:     "Remove name from status error with no details",
			input:    noDetailsError,
			expected: expectedNoDetailsError,
			obj:      builder.TestControlPlane("default", "cp1").Build(),
		},
		{
			name:     "Remove name from status error with nil object",
			input:    noDetailsError,
			expected: expectedObjectNilError,
			obj:      nil,
		},
		{
			name:     "Remove name from status error with nil object",
			input:    noDetailsError,
			expected: expectedObjectNilError,
			obj:      nil,
		},
		{
			name:     "Replace message of non status error",
			input:    nonStatusError,
			expected: expectedNonStatusError,
			obj:      builder.TestControlPlane("default", "cp1").Build(),
		},
		{
			name:     "Replace message of non status error with nil object",
			input:    nonStatusError,
			expected: expectedNilObjectNonStatusError,
			obj:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := createErrorWithoutObjectName(tt.input, tt.obj)
			g.Expect(err.Error()).To(Equal(tt.expected.Error()))
		})
	}
}
