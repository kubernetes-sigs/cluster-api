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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestReconcileTopologyReconciledCondition(t *testing.T) {
	tests := []struct {
		name                 string
		reconcileErr         error
		s                    *scope.Scope
		cluster              *clusterv1.Cluster
		wantConditionStatus  corev1.ConditionStatus
		wantConditionReason  string
		wantConditionMessage string
		wantErr              bool
	}{
		{
			name:                 "should set the condition to false if there is a reconcile error",
			reconcileErr:         errors.New("reconcile error"),
			cluster:              &clusterv1.Cluster{},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconcileFailedReason,
			wantConditionMessage: "reconcile error",
			wantErr:              false,
		},
		{
			name:    "should set the condition to false if the ClusterClass is out of date",
			cluster: &clusterv1.Cluster{},
			s: &scope.Scope{
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
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledClusterClassNotReconciledReason,
			wantConditionMessage: "ClusterClass not reconciled. If this condition persists please check ClusterClass status.",
			wantErr:              false,
		},
		{
			name:         "should set the condition to false if the there is a blocking hook",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				HookResponseTracker: func() *scope.HookResponseTracker {
					hrt := scope.NewHookResponseTracker()
					hrt.Add(runtimehooksv1.BeforeClusterUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
						CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
							CommonResponse: runtimehooksv1.CommonResponse{
								Message: "msg",
							},
							RetryAfterSeconds: int32(10),
						},
					})
					return hrt
				}(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledHookBlockingReason,
			wantConditionMessage: "hook \"BeforeClusterUpgrade\" is blocking: msg",
		},
		{
			name:         "should set the condition to false if new version is not picked up because control plane is provisioning",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.21.2").
							Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = true
					ut.ControlPlane.IsProvisioning = true
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledControlPlaneUpgradePendingReason,
			wantConditionMessage: "Control plane upgrade to v1.22.0 on hold. Control plane is completing initial provisioning",
		},
		{
			name:         "should set the condition to false if new version is not picked up because control plane is upgrading",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.21.2").
							WithReplicas(3).
							Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = true
					ut.ControlPlane.IsUpgrading = true
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledControlPlaneUpgradePendingReason,
			wantConditionMessage: "Control plane upgrade to v1.22.0 on hold. Control plane is upgrading to version v1.21.2",
		},
		{
			name:         "should set the condition to false if new version is not picked up because control plane is scaling",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.21.2").
							WithReplicas(3).
							Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = true
					ut.ControlPlane.IsScaling = true
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledControlPlaneUpgradePendingReason,
			wantConditionMessage: "Control plane upgrade to v1.22.0 on hold. Control plane is reconciling desired replicas",
		},
		{
			name:         "should set the condition to false if new version is not picked up because at least one of the machine deployment is rolling out",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.21.2").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(1),
									UpdatedReplicas:     int32(1),
									ReadyReplicas:       int32(1),
									AvailableReplicas:   int32(1),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = true
					ut.MachineDeployments.MarkRollingOut("md0-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledControlPlaneUpgradePendingReason,
			wantConditionMessage: "Control plane upgrade to v1.22.0 on hold. MachineDeployment(s) md0-abc123 are rolling out",
		},
		{
			name:         "should set the condition to false if control plane picked the new version but machine deployments did not because control plane is upgrading",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.ControlPlane.IsUpgrading = true
					ut.MachineDeployments.MarkPendingUpgrade("md0-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingReason,
			wantConditionMessage: "MachineDeployment(s) md0-abc123 upgrade to version v1.22.0 on hold. Control plane is upgrading to version v1.22.0",
		},
		{
			name:         "should set the condition to false if control plane picked the new version but machine deployments did not because control plane is scaling",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.ControlPlane.IsScaling = true
					ut.MachineDeployments.MarkPendingUpgrade("md0-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingReason,
			wantConditionMessage: "MachineDeployment(s) md0-abc123 upgrade to version v1.22.0 on hold. Control plane is reconciling desired replicas",
		},
		{
			name:         "should set the condition to true if control plane picked the new version and is upgrading but there are no machine deployments",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.ControlPlane.IsUpgrading = true
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus: corev1.ConditionTrue,
		},
		{
			name:         "should set the condition to true if control plane picked the new version and is scaling but there are no machine deployments",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.ControlPlane.IsScaling = true
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus: corev1.ConditionTrue,
		},
		{
			name:         "should set the condition to false is some machine deployments have not picked the new version because other machine deployments are rolling out (not all replicas ready)",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithVersion("v1.22.0").
								WithStatus(clusterv1.MachineDeploymentStatus{
									// MD is not ready because we don't have 2 updated, ready and available replicas.
									Replicas:            int32(2),
									UpdatedReplicas:     int32(1),
									ReadyReplicas:       int32(1),
									AvailableReplicas:   int32(1),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
						"md1": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md1-abc123").
								WithReplicas(2).
								WithVersion("v1.21.2").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.MachineDeployments.MarkRollingOut("md0-abc123")
					ut.MachineDeployments.MarkPendingUpgrade("md1-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingReason,
			wantConditionMessage: "MachineDeployment(s) md1-abc123 upgrade to version v1.22.0 on hold. MachineDeployment(s) md0-abc123 are rolling out",
		},
		{
			name:         "should set the condition to false if some machine deployments have not picked the new version because other machine deployments are rolling out (unavailable replica)",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithVersion("v1.22.0").
								WithStatus(clusterv1.MachineDeploymentStatus{
									// MD is not ready because we still have an unavailable replica.
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(1),
								}).
								Build(),
						},
						"md1": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md1-abc123").
								WithReplicas(2).
								WithVersion("v1.21.2").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.MachineDeployments.MarkRollingOut("md0-abc123")
					ut.MachineDeployments.MarkPendingUpgrade("md1-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingReason,
			wantConditionMessage: "MachineDeployment(s) md1-abc123 upgrade to version v1.22.0 on hold. MachineDeployment(s) md0-abc123 are rolling out",
		},
		{
			name:         "should set the condition to false if some machine deployments have not picked the new version because their upgrade has been deferred",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithVersion("v1.22.0").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
						"md1": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md1-abc123").
								WithReplicas(2).
								WithVersion("v1.21.2").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					ut.MachineDeployments.MarkDeferredUpgrade("md1-abc123")
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus:  corev1.ConditionFalse,
			wantConditionReason:  clusterv1.TopologyReconciledMachineDeploymentsUpgradeDeferredReason,
			wantConditionMessage: "MachineDeployment(s) md1-abc123 upgrade to version v1.22.0 deferred.",
		},
		{
			name:         "should set the condition to true if there are no reconcile errors and control plane and all machine deployments picked up the new version",
			reconcileErr: nil,
			cluster:      &clusterv1.Cluster{},
			s: &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{
					Topology: &clusterv1.Topology{
						Version: "v1.22.0",
					},
				},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
					ControlPlane: &scope.ControlPlaneState{
						Object: builder.ControlPlane("ns1", "controlplane1").
							WithVersion("v1.22.0").
							WithReplicas(3).
							Build(),
					},
					MachineDeployments: scope.MachineDeploymentsStateMap{
						"md0": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md0-abc123").
								WithReplicas(2).
								WithVersion("v1.22.0").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(1),
									UpdatedReplicas:     int32(1),
									ReadyReplicas:       int32(1),
									AvailableReplicas:   int32(1),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
						"md1": &scope.MachineDeploymentState{
							Object: builder.MachineDeployment("ns1", "md1-abc123").
								WithReplicas(2).
								WithVersion("v1.22.0").
								WithStatus(clusterv1.MachineDeploymentStatus{
									Replicas:            int32(2),
									UpdatedReplicas:     int32(2),
									ReadyReplicas:       int32(2),
									AvailableReplicas:   int32(2),
									UnavailableReplicas: int32(0),
								}).
								Build(),
						},
					},
				},
				UpgradeTracker: func() *scope.UpgradeTracker {
					ut := scope.NewUpgradeTracker()
					ut.ControlPlane.PendingUpgrade = false
					return ut
				}(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			},
			wantConditionStatus: corev1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &Reconciler{}
			err := r.reconcileTopologyReconciledCondition(tt.s, tt.cluster, tt.reconcileErr)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())

				actualCondition := conditions.Get(tt.cluster, clusterv1.TopologyReconciledCondition)
				g.Expect(actualCondition.Status).To(Equal(tt.wantConditionStatus))
				g.Expect(actualCondition.Reason).To(Equal(tt.wantConditionReason))
				g.Expect(actualCondition.Message).To(Equal(tt.wantConditionMessage))
			}
		})
	}
}

func TestComputeMachineDeploymentNameList(t *testing.T) {
	tests := []struct {
		name     string
		mdList   []string
		expected string
	}{
		{
			name:     "mdList with 4 names",
			mdList:   []string{"md-1", "md-2", "md-3", "md-4"},
			expected: "md-1, md-2, md-3, md-4",
		},
		{
			name:     "mdList with 5 names",
			mdList:   []string{"md-1", "md-2", "md-3", "md-4", "md-5"},
			expected: "md-1, md-2, md-3, md-4, md-5",
		},
		{
			name:     "mdList with 6 names is shortened",
			mdList:   []string{"md-1", "md-2", "md-3", "md-4", "md-5", "md-6"},
			expected: "md-1, md-2, md-3, md-4, md-5, ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(computeMachineDeploymentNameList(tt.mdList)).To(Equal(tt.expected))
		})
	}
}
