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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestReconcileTopologyReconciledCondition(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	deletionTime := metav1.Unix(0, 0)
	tests := []struct {
		name                        string
		reconcileErr                error
		s                           *scope.Scope
		machines                    []*clusterv1.Machine
		wantV1Beta1ConditionStatus  corev1.ConditionStatus
		wantV1Beta1ConditionReason  string
		wantV1Beta1ConditionMessage string
		wantConditionStatus         metav1.ConditionStatus
		wantConditionReason         string
		wantConditionMessage        string
		wantErr                     bool
	}{
		// Reconcile error

		{
			name:         "should set the condition to false if there is a reconcile error",
			reconcileErr: errors.New("reconcile error"),
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
				},
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.TopologyReconcileFailedV1Beta1Reason,
			wantV1Beta1ConditionMessage: "reconcile error",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconciledFailedReason,
			wantConditionMessage:        "reconcile error",
			wantErr:                     false,
		},

		// Paused

		{
			name: "should set the TopologyReconciledCondition to False if spec.paused is set to true",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							Paused: ptr.To(true),
						},
					},
				},
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.TopologyReconciledPausedV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster spec.paused is set to true",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconcilePausedReason,
			wantConditionMessage:        "Cluster spec.paused is set to true",
		},
		{
			name: "should set the TopologyReconciledCondition to False if cluster.x-k8s.io/paused annotation is set to true",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								clusterv1.PausedAnnotation: "",
							},
						},
					},
				},
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.TopologyReconciledPausedV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster has the cluster.x-k8s.io/paused annotation",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconcilePausedReason,
			wantConditionMessage:        "Cluster has the cluster.x-k8s.io/paused annotation",
		},
		{
			name: "should set the TopologyReconciledCondition to False if spec.paused is set to true and if cluster.x-k8s.io/paused annotation is set to true",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								clusterv1.PausedAnnotation: "",
							},
						},
						Spec: clusterv1.ClusterSpec{
							Paused: ptr.To(true),
						},
					},
				},
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.TopologyReconciledPausedV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster spec.paused is set to true, Cluster has the cluster.x-k8s.io/paused annotation",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconcilePausedReason,
			wantConditionMessage:        "Cluster spec.paused is set to true, Cluster has the cluster.x-k8s.io/paused annotation",
		},

		// Delete
		{
			name: "should set the TopologyReconciledCondition to False if the cluster has been deleted but the BeforeClusterDelete hook is blocking",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							DeletionTimestamp: &deletionTime,
						},
					},
				},
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterDelete, &runtimehooksv1.BeforeClusterDeleteResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.DeletingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is deleting. Following hooks are blocking delete: BeforeClusterDelete: msg",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconciledDeletingReason,
			wantConditionMessage:        "Cluster is deleting. Following hooks are blocking delete: BeforeClusterDelete: msg",
		},
		{
			name: "should set the TopologyReconciledCondition to False if the cluster has been deleted",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							DeletionTimestamp: &deletionTime,
						},
					},
				},
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterDelete, &runtimehooksv1.BeforeClusterDeleteResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{},
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.DeletingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is deleting",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconciledDeletingReason,
			wantConditionMessage:        "Cluster is deleting",
		},

		// Cluster Class out of date

		{
			name: "should set the condition to false if the ClusterClass is out of date",
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
				},
				Blueprint: &scope.ClusterBlueprint{
					ClusterClass: &clusterv1.ClusterClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "class1",
							Generation: 10,
						},
						Status: clusterv1.ClusterClassStatus{
							ObservedGeneration: 999,
						},
					},
				},
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterClassNotReconciledV1Beta1Reason,
			wantV1Beta1ConditionMessage: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
				".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledReason,
			wantConditionMessage: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
				".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			wantErr: false,
		},

		// BeforeClusterCreate hook is blocking

		{
			name:         "should set the condition to false if BeforeClusterCreate hook is blocking",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
				},
				UpgradeTracker: scope.NewUpgradeTracker(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterCreate, &runtimehooksv1.BeforeClusterCreateResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus:  corev1.ConditionFalse,
			wantV1Beta1ConditionReason:  clusterv1.TopologyReconciledClusterCreatingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Following hooks are blocking Cluster topology creation: BeforeClusterCreate: msg",
			wantConditionStatus:         metav1.ConditionFalse,
			wantConditionReason:         clusterv1.ClusterTopologyReconciledClusterCreatingReason,
			wantConditionMessage:        "Following hooks are blocking Cluster topology creation: BeforeClusterCreate: msg",
		},

		// Upgrade

		// First upgrade step (CP only, MD are skipping this step)
		{
			name:         "should set the condition to false if control plane is pending upgrade and BeforeClusterUpgrade hook is blocking via an annotation",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								clusterv1.BeforeClusterUpgradeHookAnnotationPrefix + "/test": "true",
							},
						},
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.20.5").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.21.2", "v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: fmt.Sprintf("annotation %s is set", clusterv1.BeforeClusterUpgradeHookAnnotationPrefix+"/test"),
							},
							RetryAfterSeconds: int32(20 * 60),
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeClusterUpgrade: annotation before-upgrade.hook.cluster.cluster.x-k8s.io/test is set\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeClusterUpgrade: annotation before-upgrade.hook.cluster.cluster.x-k8s.io/test is set\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is pending upgrade and BeforeClusterUpgrade hook is blocking",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.20.5").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.21.2", "v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeClusterUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeClusterUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is pending upgrade and BeforeControlPlaneUpgrade hook is blocking (first upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.20.5").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.21.2", "v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeControlPlaneUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.21.2, v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is starting upgrade (first upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.21.2").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.IsStartingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.21.2 (v1.22.0 pending)\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.21.2 (v1.22.0 pending)\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is upgrading (first upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.21.2").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.IsUpgrading = true
					ut.ControlPlane.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.21.2 (v1.22.0 pending)\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.21.2 (v1.22.0 pending)\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is upgraded and AfterControlPlaneUpgrade hook is blocking (first upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.21.2").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.AfterControlPlaneUpgrade, &runtimehooksv1.AfterControlPlaneUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		// Second upgrade step (CP and MachineDeployments)
		{
			name:         "should set the condition to false if control plane is upgrading and BeforeControlPlaneUpgrade hook is blocking (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.21.2").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsPendingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeControlPlaneUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeControlPlaneUpgrade: msg\n" +
				"  * GenericControlPlane pending upgrade to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is starting upgrade (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsStartingUpgrade = true
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is upgrading (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsUpgrading = true
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is upgraded and AfterControlPlaneUpgrade hook is blocking (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.AfterControlPlaneUpgrade, &runtimehooksv1.AfterControlPlaneUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterControlPlaneUpgrade: msg\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterControlPlaneUpgrade: msg\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane is upgraded and BeforeWorkersUpgrade hook is blocking (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeWorkersUpgrade, &runtimehooksv1.BeforeWorkersUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeWorkersUpgrade: msg\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: BeforeWorkersUpgrade: msg\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if MachineDeployments are upgrading (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkUpgrading("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md1 upgrading to version v1.22.0\n" +
				"  * MachineDeployments md2, md3, md4 pending upgrade to version v1.22.0",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md1 upgrading to version v1.22.0\n" +
				"  * MachineDeployments md2, md3, md4 pending upgrade to version v1.22.0",
		},
		{
			name:         "should set the condition to false if MachineDeployments are upgraded and AfterWorkersUpgrade hook is blocking (second upgrade step)",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
							},
						},
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{}
					return ut
				}(),
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.AfterWorkersUpgrade, &runtimehooksv1.AfterWorkersUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: 10,
						},
					})
					return hrt
				}(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterWorkersUpgrade: msg",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * Following hooks are blocking upgrade: AfterWorkersUpgrade: msg",
		},
		// TODO(chained): uncomment this as soon as AfterClusterUpgrade will be blocking
		/*
			{
				name:         "should set the condition to false if AfterClusterUpgrade hook is blocking",
				reconcileErr: nil,
				s: &scope.Scope{
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{
								ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
								InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
								Topology: clusterv1.Topology{
									Version: "v1.22.0",
								},
							},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
						},
					},
					UpgradeTracker:  scope.NewUpgradeTracker(),
					HookResponseTracker: func() *scope.HookResponseTracker {
						hrt := scope.NewHookResponseTracker()
						hrt.Add(runtimehooksv1.AfterClusterUpgrade, &runtimehooksv1.AfterClusterUpgradeResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{
									Message: "msg",
								},
								RetryAfterSeconds: 10,
							},
						})
						return hrt
					}(),
				},
				wantV1Beta1ConditionStatus: corev1.ConditionFalse,
				wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
				wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
					"  * Following hooks are blocking upgrade: AfterClusterUpgrade: msg",
				wantConditionStatus: metav1.ConditionFalse,
				wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
				wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
					"  * Following hooks are blocking upgrade: AfterClusterUpgrade: msg",
			},
		*/

		// Hold & defer upgrade

		{
			name:         "should report deferred MD upgrades",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkDeferredUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md4 pending upgrade to version v1.22.0\n" +
				"  * MachineDeployment md3 upgrade to version v1.22.0 deferred using defer-upgrade or hold-upgrade-sequence annotations",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md4 pending upgrade to version v1.22.0\n" +
				"  * MachineDeployment md3 upgrade to version v1.22.0 deferred using defer-upgrade or hold-upgrade-sequence annotations",
		},
		{
			name:         "should report when deferred MD upgrades are blocking further progress",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkDeferredUpgrade("md2")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledMachineDeploymentsUpgradeDeferredV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md2 upgrade to version v1.22.0 deferred using defer-upgrade or hold-upgrade-sequence annotations",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * MachineDeployment md2 upgrade to version v1.22.0 deferred using defer-upgrade or hold-upgrade-sequence annotations",
		},

		// Create deferred
		{
			name:         "should report MachineDeployment creation deferred while CP is upgrading",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.IsUpgrading = true
					ut.ControlPlane.UpgradePlan = []string{}
					ut.MachineDeployments.UpgradePlan = []string{"v1.22.0"}
					ut.MachineDeployments.MarkPendingUpgrade("md1")
					ut.MachineDeployments.MarkPendingUpgrade("md2")
					ut.MachineDeployments.MarkPendingUpgrade("md3")
					ut.MachineDeployments.MarkPendingUpgrade("md4")
					ut.MachineDeployments.MarkPendingCreate("md5")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionFalse,
			wantV1Beta1ConditionReason: clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason,
			wantV1Beta1ConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0\n" +
				"  * MachineDeployment md5 creation deferred while control plane upgrade is in progress",
			wantConditionStatus: metav1.ConditionFalse,
			wantConditionReason: clusterv1.ClusterTopologyReconciledClusterUpgradingReason,
			wantConditionMessage: "Cluster is upgrading to v1.22.0\n" +
				"  * GenericControlPlane upgrading to version v1.22.0\n" +
				"  * MachineDeployments md1, md2, md3, ... (1 more) pending upgrade to version v1.22.0\n" +
				"  * MachineDeployment md5 creation deferred while control plane upgrade is in progress",
		},

		{
			name:         "should set the condition to true if cluster is not upgrading",
			reconcileErr: nil,
			s: &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef:   clusterv1.ContractVersionedObjectReference{Name: "controlplane1"},
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "infra1"},
							Topology: clusterv1.Topology{
								Version: "v1.22.0",
							},
						},
					},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").WithVersion("v1.22.0").Build(),
					},
				},
				UpgradeTracker:      scope.NewUpgradeTracker(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantV1Beta1ConditionStatus: corev1.ConditionTrue,
			wantConditionStatus:        metav1.ConditionTrue,
			wantConditionReason:        clusterv1.ClusterTopologyReconcileSucceededReason,
			wantConditionMessage:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

			objs := []client.Object{}
			if tt.s != nil && tt.s.Current != nil {
				for _, md := range tt.s.Current.MachineDeployments {
					objs = append(objs, md.Object)
				}
				for _, mp := range tt.s.Current.MachinePools {
					objs = append(objs, mp.Object)
				}
			}
			for _, m := range tt.machines {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			r := &Reconciler{Client: fakeClient}
			err := r.reconcileTopologyReconciledCondition(tt.s, tt.s.Current.Cluster, tt.reconcileErr)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())

				actualV1Beta1Condition := v1beta1conditions.Get(tt.s.Current.Cluster, clusterv1.TopologyReconciledV1Beta1Condition)
				g.Expect(actualV1Beta1Condition).ToNot(BeNil())
				g.Expect(actualV1Beta1Condition.Status).To(BeEquivalentTo(tt.wantV1Beta1ConditionStatus))
				g.Expect(actualV1Beta1Condition.Reason).To(BeEquivalentTo(tt.wantV1Beta1ConditionReason))
				g.Expect(actualV1Beta1Condition.Message).To(BeEquivalentTo(tt.wantV1Beta1ConditionMessage))

				actualCondition := conditions.Get(tt.s.Current.Cluster, clusterv1.ClusterTopologyReconciledCondition)
				g.Expect(actualCondition).ToNot(BeNil())
				g.Expect(actualCondition.Status).To(BeEquivalentTo(tt.wantConditionStatus))
				g.Expect(actualCondition.Reason).To(BeEquivalentTo(tt.wantConditionReason))
				g.Expect(actualCondition.Message).To(BeEquivalentTo(tt.wantConditionMessage))
			}
		})
	}
}

func TestComputeNameList(t *testing.T) {
	tests := []struct {
		name     string
		mdList   []string
		expected string
	}{
		{
			name:     "mdList with 2 names",
			mdList:   []string{"md-1", "md-2"},
			expected: "md-1, md-2",
		},
		{
			name:     "mdList with 3 names",
			mdList:   []string{"md-1", "md-2", "md-3"},
			expected: "md-1, md-2, md-3",
		},
		{
			name:     "mdList with 4 names is shortened",
			mdList:   []string{"md-1", "md-2", "md-3", "md-4"},
			expected: "md-1, md-2, md-3, ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(computeNameList(tt.mdList)).To(Equal(tt.expected))
		})
	}
}
