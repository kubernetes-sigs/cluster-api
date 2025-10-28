/*
Copyright 2025 The Kubernetes Authors.

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

package desiredstate

import (
	"maps"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestComputeControlPlaneVersion_LifecycleHooksSequences(t *testing.T) {
	var testGVKs = []schema.GroupVersionKind{
		{
			Group:   "refAPIGroup1",
			Kind:    "refKind1",
			Version: "v1beta4",
		},
	}

	apiVersionGetter := func(gk schema.GroupKind) (string, error) {
		for _, gvk := range testGVKs {
			if gvk.GroupKind() == gk {
				return schema.GroupVersion{
					Group:   gk.Group,
					Version: gvk.Version,
				}.String(), nil
			}
		}
		return "", errors.Errorf("unknown GroupVersionKind: %v", gk)
	}
	clusterv1beta1.SetAPIVersionGetter(apiVersionGetter)

	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	beforeClusterUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterUpgrade)
	if err != nil {
		panic("unable to compute GVH")
	}
	blockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	nonBlockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	afterControlPlaneUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.AfterControlPlaneUpgrade)
	if err != nil {
		panic("unable to compute GVH")
	}

	blockingAfterControlPlaneUpgradeResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	nonBlockingAfterControlPlaneUpgradeResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	tests := []struct {
		name                                string
		topologyVersion                     string
		pendingHookAnnotation               string
		controlPlaneObj                     *unstructured.Unstructured
		controlPlaneUpgradePlan             []string
		machineDeploymentsUpgradePlan       []string
		machinePoolsUpgradePlan             []string
		upgradingMachineDeployments         []string
		upgradingMachinePools               []string
		wantBeforeClusterUpgradeRequest     *runtimehooksv1.BeforeClusterUpgradeRequest
		beforeClusterUpgradeResponse        *runtimehooksv1.BeforeClusterUpgradeResponse
		wantAfterControlPlaneUpgradeRequest *runtimehooksv1.AfterControlPlaneUpgradeRequest
		afterControlPlaneUpgradeResponse    *runtimehooksv1.AfterControlPlaneUpgradeResponse
		wantVersion                         string
		wantIsPendingUpgrade                bool
		wantIsStartingUpgrade               bool
		wantIsWaitingForWorkersUpgrade      bool
		wantPendingHookAnnotation           string
	}{
		// Upgrade cluster with CP, MD, MP (upgrade by one minor)

		{
			name:            "no hook called before starting the upgrade",
			topologyVersion: "v1.2.2",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			wantVersion: "v1.2.2",
		},
		{
			name:            "when an upgrade starts: call the BeforeClusterUpgrade hook, blocking answer",
			topologyVersion: "v1.2.3", // changed from previous step
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.2.3"},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantBeforeClusterUpgradeRequest: &runtimehooksv1.BeforeClusterUpgradeRequest{
				FromKubernetesVersion: "v1.2.2",
				ToKubernetesVersion:   "v1.2.3",
				ControlPlaneUpgrades:  toUpgradeStep([]string{"v1.2.3"}),
				WorkersUpgrades:       toUpgradeStep([]string{"v1.2.3"}),
			},
			beforeClusterUpgradeResponse: blockingBeforeClusterUpgradeResponse,
			wantVersion:                  "v1.2.2",
			wantIsPendingUpgrade:         true,
		},
		{
			name:            "when an upgrade starts: pick up a new version when BeforeClusterUpgrade hook unblocks",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.2.3"},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantBeforeClusterUpgradeRequest: &runtimehooksv1.BeforeClusterUpgradeRequest{
				FromKubernetesVersion: "v1.2.2",
				ToKubernetesVersion:   "v1.2.3",
				ControlPlaneUpgrades:  toUpgradeStep([]string{"v1.2.3"}),
				WorkersUpgrades:       toUpgradeStep([]string{"v1.2.3"}),
			},
			beforeClusterUpgradeResponse: nonBlockingBeforeClusterUpgradeResponse,
			wantVersion:                  "v1.2.3", // changed from previous step
			wantIsStartingUpgrade:        true,
			wantPendingHookAnnotation:    "AfterClusterUpgrade,AfterControlPlaneUpgrade", // changed from previous step
		},
		{
			name:                  "when control plane is upgrading: do not call hooks",
			topologyVersion:       "v1.2.3",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.2.3"},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantVersion:                   "v1.2.3",
			wantPendingHookAnnotation:     "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "after control plane is upgraded: call the AfterControlPlaneUpgrade hook, blocking answer",
			topologyVersion:       "v1.2.3",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.3", // changed from previous step
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantAfterControlPlaneUpgradeRequest: &runtimehooksv1.AfterControlPlaneUpgradeRequest{
				KubernetesVersion:    "v1.2.3",
				ControlPlaneUpgrades: toUpgradeStep([]string{}),
				WorkersUpgrades:      toUpgradeStep([]string{"v1.2.3"}),
			},
			afterControlPlaneUpgradeResponse: blockingAfterControlPlaneUpgradeResponse,
			wantVersion:                      "v1.2.3",
			wantPendingHookAnnotation:        "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "after control plane is upgraded: AfterControlPlaneUpgrade hook unblocks",
			topologyVersion:       "v1.2.3",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.3",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantAfterControlPlaneUpgradeRequest: &runtimehooksv1.AfterControlPlaneUpgradeRequest{
				KubernetesVersion:    "v1.2.3",
				ControlPlaneUpgrades: toUpgradeStep([]string{}),
				WorkersUpgrades:      toUpgradeStep([]string{"v1.2.3"}),
			},
			afterControlPlaneUpgradeResponse: nonBlockingAfterControlPlaneUpgradeResponse,
			wantVersion:                      "v1.2.3",
			wantPendingHookAnnotation:        "AfterClusterUpgrade", // changed from previous step
		},
		{
			name:                  "when machine deployment are upgrading: do not call hooks",
			topologyVersion:       "v1.2.3",
			pendingHookAnnotation: "AfterClusterUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.3",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{"v1.2.3"},
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantVersion:                   "v1.2.3",
			wantPendingHookAnnotation:     "AfterClusterUpgrade",
		},
		{
			name:                  "when machine pools are upgrading: do not call hooks",
			topologyVersion:       "v1.2.3",
			pendingHookAnnotation: "AfterClusterUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.3",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{}, // changed from previous step
			machinePoolsUpgradePlan:       []string{"v1.2.3"},
			wantVersion:                   "v1.2.3",
			wantPendingHookAnnotation:     "AfterClusterUpgrade",
		},
		// Note: After MP upgrade completes, the AfterClusterUpgrade is called from reconcile_state.go

		// Upgrade cluster with CP, MD (upgrade by two minors, workers skip the first one)

		{
			name:            "no hook called before starting the upgrade",
			topologyVersion: "v1.2.2",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			wantVersion: "v1.2.2",
		},
		{
			name:            "when an upgrade to the first minor starts: call the BeforeClusterUpgrade hook, blocking answer",
			topologyVersion: "v1.4.4", // changed from previous step
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.3.3", "v1.4.4"},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantBeforeClusterUpgradeRequest: &runtimehooksv1.BeforeClusterUpgradeRequest{
				FromKubernetesVersion: "v1.2.2",
				ToKubernetesVersion:   "v1.4.4",
				ControlPlaneUpgrades:  toUpgradeStep([]string{"v1.3.3", "v1.4.4"}),
				WorkersUpgrades:       toUpgradeStep([]string{"v1.4.4"}),
			},
			beforeClusterUpgradeResponse: blockingBeforeClusterUpgradeResponse,
			wantVersion:                  "v1.2.2",
			wantIsPendingUpgrade:         true,
		},
		{
			name:            "when an upgrade to the first minor starts: BeforeClusterUpgrade hook unblocks, pick up the new version",
			topologyVersion: "v1.4.4",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.3.3", "v1.4.4"},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantBeforeClusterUpgradeRequest: &runtimehooksv1.BeforeClusterUpgradeRequest{
				FromKubernetesVersion: "v1.2.2",
				ToKubernetesVersion:   "v1.4.4",
				ControlPlaneUpgrades:  toUpgradeStep([]string{"v1.3.3", "v1.4.4"}),
				WorkersUpgrades:       toUpgradeStep([]string{"v1.4.4"}),
			},
			beforeClusterUpgradeResponse: nonBlockingBeforeClusterUpgradeResponse,
			wantVersion:                  "v1.3.3", // changed from previous step
			wantIsStartingUpgrade:        true,
			wantPendingHookAnnotation:    "AfterClusterUpgrade,AfterControlPlaneUpgrade", // changed from previous step
		},
		{
			name:                  "when control plane is upgrading to the first minor: do not call hooks",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.3.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.2",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.4.4"}, // changed from previous step
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantVersion:                   "v1.3.3",
			wantIsPendingUpgrade:          true,
			wantPendingHookAnnotation:     "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "after control plane is upgraded to the first minor: call the AfterControlPlaneUpgrade hook, blocking answer",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.3.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.3.3", // changed from previous step
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.4.4"},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantAfterControlPlaneUpgradeRequest: &runtimehooksv1.AfterControlPlaneUpgradeRequest{
				KubernetesVersion:    "v1.3.3",
				ControlPlaneUpgrades: toUpgradeStep([]string{"v1.4.4"}),
				WorkersUpgrades:      toUpgradeStep([]string{"v1.4.4"}),
			},
			afterControlPlaneUpgradeResponse: blockingAfterControlPlaneUpgradeResponse,
			wantVersion:                      "v1.3.3",
			wantIsPendingUpgrade:             true,
			wantPendingHookAnnotation:        "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "when an upgrade to the second minor starts: pick up a new version when AfterControlPlaneUpgrade hook unblocks",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.3.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.3.3",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{"v1.4.4"},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantAfterControlPlaneUpgradeRequest: &runtimehooksv1.AfterControlPlaneUpgradeRequest{
				KubernetesVersion:    "v1.3.3",
				ControlPlaneUpgrades: toUpgradeStep([]string{"v1.4.4"}),
				WorkersUpgrades:      toUpgradeStep([]string{"v1.4.4"}),
			},
			afterControlPlaneUpgradeResponse: nonBlockingAfterControlPlaneUpgradeResponse,
			wantVersion:                      "v1.4.4", // changed from previous step
			wantIsStartingUpgrade:            true,
			wantPendingHookAnnotation:        "AfterClusterUpgrade,AfterControlPlaneUpgrade", // changed from previous step
		},
		{
			name:                  "when control plane is upgrading to the second minor: do not call hooks",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.4.4",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.3.3",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{}, // changed from previous step
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantVersion:                   "v1.4.4",
			wantPendingHookAnnotation:     "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "after control plane is upgraded to the second minor: call the AfterControlPlaneUpgrade hook, blocking answer",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.4.4",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.4.4", // changed from previous step
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantAfterControlPlaneUpgradeRequest: &runtimehooksv1.AfterControlPlaneUpgradeRequest{
				KubernetesVersion:    "v1.4.4",
				ControlPlaneUpgrades: toUpgradeStep([]string{}),
				WorkersUpgrades:      toUpgradeStep([]string{"v1.4.4"}),
			},
			afterControlPlaneUpgradeResponse: blockingAfterControlPlaneUpgradeResponse,
			wantVersion:                      "v1.4.4",
			wantPendingHookAnnotation:        "AfterClusterUpgrade,AfterControlPlaneUpgrade",
		},
		{
			name:                  "when machine deployment are upgrading to the second minor: do not call hooks",
			topologyVersion:       "v1.4.4",
			pendingHookAnnotation: "AfterClusterUpgrade",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.4.4",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.4.4",
				}).
				Build(),
			controlPlaneUpgradePlan:       []string{},
			machineDeploymentsUpgradePlan: []string{"v1.4.4"},
			machinePoolsUpgradePlan:       []string{},
			wantVersion:                   "v1.4.4",
			wantPendingHookAnnotation:     "AfterClusterUpgrade",
		},
		// Note: After MD upgrade completes, the AfterClusterUpgrade is called from reconcile_state.go
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: ptr.To[int32](2),
					},
				}},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							// Add managedFields and annotations that should be cleaned up before the Cluster is sent to the RuntimeExtension.
							ManagedFields: []metav1.ManagedFieldsEntry{
								{
									APIVersion: builder.InfrastructureGroupVersion.String(),
									Manager:    "manager",
									Operation:  "Apply",
									Time:       ptr.To(metav1.Now()),
									FieldsType: "FieldsV1",
								},
							},
							Annotations: map[string]string{
								"fizz":                             "buzz",
								corev1.LastAppliedConfigAnnotation: "should be cleaned up",
								conversion.DataAnnotation:          "should be cleaned up",
							},
						},
						// Add some more fields to check that conversion implemented when calling RuntimeExtension are properly handled.
						Spec: clusterv1.ClusterSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: "refAPIGroup1",
								Kind:     "refKind1",
								Name:     "refName1",
							}},
					},
					ControlPlane: &scope.ControlPlaneState{Object: tt.controlPlaneObj},
				},
				UpgradeTracker:      scope.NewUpgradeTracker(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			}
			if tt.pendingHookAnnotation != "" {
				if s.Current.Cluster.Annotations == nil {
					s.Current.Cluster.Annotations = map[string]string{}
				}
				s.Current.Cluster.Annotations[runtimev1.PendingHooksAnnotation] = tt.pendingHookAnnotation
			}
			if len(tt.controlPlaneUpgradePlan) > 0 {
				s.UpgradeTracker.ControlPlane.UpgradePlan = tt.controlPlaneUpgradePlan
			}
			if len(tt.machineDeploymentsUpgradePlan) > 0 {
				s.UpgradeTracker.MachineDeployments.UpgradePlan = tt.machineDeploymentsUpgradePlan
			}
			if len(tt.machinePoolsUpgradePlan) > 0 {
				s.UpgradeTracker.MachinePools.UpgradePlan = tt.machinePoolsUpgradePlan
			}
			if len(tt.upgradingMachineDeployments) > 0 {
				s.UpgradeTracker.MachineDeployments.MarkUpgrading(tt.upgradingMachineDeployments...)
			}
			if len(tt.upgradingMachinePools) > 0 {
				s.UpgradeTracker.MachinePools.MarkUpgrading(tt.upgradingMachinePools...)
			}

			hooksCalled := sets.Set[string]{}
			validateHookCall := func(request runtimehooksv1.RequestObject) error {
				switch request := request.(type) {
				case *runtimehooksv1.BeforeClusterUpgradeRequest:
					hooksCalled.Insert("BeforeClusterUpgrade")
					if err := validateHookRequest(request, tt.wantBeforeClusterUpgradeRequest); err != nil {
						return err
					}
				case *runtimehooksv1.AfterControlPlaneUpgradeRequest:
					hooksCalled.Insert("AfterControlPlaneUpgrade")
					if err := validateHookRequest(request, tt.wantAfterControlPlaneUpgradeRequest); err != nil {
						return err
					}
				default:
					return errors.Errorf("unhandled request type %T", request)
				}
				return validateClusterParameter(s.Current.Cluster)(request)
			}

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					beforeClusterUpgradeGVH:     tt.beforeClusterUpgradeResponse,
					afterControlPlaneUpgradeGVH: tt.afterControlPlaneUpgradeResponse,
				}).
				WithCallAllExtensionValidations(validateHookCall).
				Build()

			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(s.Current.Cluster).Build()

			r := &generator{
				Client:        fakeClient,
				RuntimeClient: runtimeClient,
			}
			version, err := r.computeControlPlaneVersion(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(version).To(Equal(tt.wantVersion), "unexpected version")
			g.Expect(s.UpgradeTracker.ControlPlane.IsPendingUpgrade).To(Equal(tt.wantIsPendingUpgrade), "unexpected IsPendingUpgrade")
			g.Expect(s.UpgradeTracker.ControlPlane.IsStartingUpgrade).To(Equal(tt.wantIsStartingUpgrade), "unexpected IsStartingUpgrade")
			g.Expect(s.UpgradeTracker.ControlPlane.IsWaitingForWorkersUpgrade).To(Equal(tt.wantIsWaitingForWorkersUpgrade), "unexpected IsWaitingForWorkersUpgrade")

			// check call received
			g.Expect(hooksCalled.Has("BeforeClusterUpgrade")).To(Equal(tt.wantBeforeClusterUpgradeRequest != nil), "Unexpected call/missing call to BeforeClusterUpgrade")
			g.Expect(hooksCalled.Has("AfterControlPlaneUpgrade")).To(Equal(tt.wantAfterControlPlaneUpgradeRequest != nil), "Unexpected call/missing call to AfterControlPlaneUpgrade")

			// check intent to call hooks
			if tt.wantPendingHookAnnotation != "" {
				g.Expect(s.Current.Cluster.Annotations).To(HaveKeyWithValue(runtimev1.PendingHooksAnnotation, tt.wantPendingHookAnnotation), "Unexpected PendingHookAnnotation")
			} else {
				g.Expect(s.Current.Cluster.Annotations).ToNot(HaveKey(runtimev1.PendingHooksAnnotation), "Unexpected PendingHookAnnotation")
			}
		})
	}
}

func validateHookRequest(request runtimehooksv1.RequestObject, wantRequest runtimehooksv1.RequestObject) error {
	if request, ok := request.(*runtimehooksv1.BeforeClusterUpgradeRequest); ok {
		if wantRequest, ok := wantRequest.(*runtimehooksv1.BeforeClusterUpgradeRequest); ok && wantRequest != nil {
			if wantRequest.FromKubernetesVersion != request.FromKubernetesVersion {
				return errors.Errorf("unexpected BeforeClusterUpgradeRequest.FromKubernetesVersion version %s, want %s", request.FromKubernetesVersion, wantRequest.FromKubernetesVersion)
			}
			if wantRequest.ToKubernetesVersion != request.ToKubernetesVersion {
				return errors.Errorf("unexpected BeforeClusterUpgradeRequest.ToKubernetes version %s, want %s", request.ToKubernetesVersion, wantRequest.ToKubernetesVersion)
			}
			if !reflect.DeepEqual(wantRequest.ControlPlaneUpgrades, request.ControlPlaneUpgrades) {
				return errors.Errorf("unexpected BeforeClusterUpgradeRequest.ControlPlaneUpgrades %s, want %s", request.ControlPlaneUpgrades, wantRequest.ControlPlaneUpgrades)
			}
			if !reflect.DeepEqual(wantRequest.WorkersUpgrades, request.WorkersUpgrades) {
				return errors.Errorf("unexpected BeforeClusterUpgradeRequest.WorkersUpgrades %s, want %s", request.WorkersUpgrades, wantRequest.WorkersUpgrades)
			}
		} else {
			return errors.Errorf("got an unexpected request of type %T", request)
		}
	}
	if request, ok := request.(*runtimehooksv1.BeforeControlPlaneUpgradeRequest); ok {
		if wantRequest, ok := wantRequest.(*runtimehooksv1.BeforeControlPlaneUpgradeRequest); ok && wantRequest != nil {
			if wantRequest.FromKubernetesVersion != request.FromKubernetesVersion {
				return errors.Errorf("unexpected BeforeControlPlaneUpgradeRequest.FromKubernetesVersion version %s, want %s", request.FromKubernetesVersion, wantRequest.FromKubernetesVersion)
			}
			if wantRequest.ToKubernetesVersion != request.ToKubernetesVersion {
				return errors.Errorf("unexpected BeforeControlPlaneUpgradeRequest.ToKubernetes version %s, want %s", request.ToKubernetesVersion, wantRequest.ToKubernetesVersion)
			}
			if !reflect.DeepEqual(wantRequest.ControlPlaneUpgrades, request.ControlPlaneUpgrades) {
				return errors.Errorf("unexpected BeforeControlPlaneUpgradeRequest.ControlPlaneUpgrades %s, want %s", request.ControlPlaneUpgrades, wantRequest.ControlPlaneUpgrades)
			}
			if !reflect.DeepEqual(wantRequest.WorkersUpgrades, request.WorkersUpgrades) {
				return errors.Errorf("unexpected BeforeControlPlaneUpgradeRequest.WorkersUpgrades %s, want %s", request.WorkersUpgrades, wantRequest.WorkersUpgrades)
			}
		} else {
			return errors.Errorf("got an unexpected request of type %T", request)
		}
	}
	if request, ok := request.(*runtimehooksv1.BeforeWorkersUpgradeRequest); ok {
		if wantRequest, ok := wantRequest.(*runtimehooksv1.BeforeWorkersUpgradeRequest); ok && wantRequest != nil {
			if wantRequest.FromKubernetesVersion != request.FromKubernetesVersion {
				return errors.Errorf("unexpected BeforeWorkersUpgradeRequest.FromKubernetesVersion version %s, want %s", request.FromKubernetesVersion, wantRequest.FromKubernetesVersion)
			}
			if wantRequest.ToKubernetesVersion != request.ToKubernetesVersion {
				return errors.Errorf("unexpected BeforeWorkersUpgradeRequest.ToKubernetes version %s, want %s", request.ToKubernetesVersion, wantRequest.ToKubernetesVersion)
			}
			if !reflect.DeepEqual(wantRequest.ControlPlaneUpgrades, request.ControlPlaneUpgrades) {
				return errors.Errorf("unexpected BeforeWorkersUpgradeRequest.ControlPlaneUpgrades %s, want %s", request.ControlPlaneUpgrades, wantRequest.ControlPlaneUpgrades)
			}
			if !reflect.DeepEqual(wantRequest.WorkersUpgrades, request.WorkersUpgrades) {
				return errors.Errorf("unexpected BeforeWorkersUpgradeRequest.WorkersUpgrades %s, want %s", request.WorkersUpgrades, wantRequest.WorkersUpgrades)
			}
		} else {
			return errors.Errorf("got an unexpected request of type %T", request)
		}
	}
	if request, ok := request.(*runtimehooksv1.AfterControlPlaneUpgradeRequest); ok {
		if wantRequest, ok := wantRequest.(*runtimehooksv1.AfterControlPlaneUpgradeRequest); ok && wantRequest != nil {
			if wantRequest.KubernetesVersion != request.KubernetesVersion {
				return errors.Errorf("unexpected AfterControlPlaneUpgradeRequest.Kubernetes version %s, want %s", request.KubernetesVersion, wantRequest.KubernetesVersion)
			}
			if !reflect.DeepEqual(wantRequest.ControlPlaneUpgrades, request.ControlPlaneUpgrades) {
				return errors.Errorf("unexpected AfterControlPlaneUpgradeRequest.ControlPlaneUpgrades %s, want %s", request.ControlPlaneUpgrades, wantRequest.ControlPlaneUpgrades)
			}
			if !reflect.DeepEqual(wantRequest.WorkersUpgrades, request.WorkersUpgrades) {
				return errors.Errorf("unexpected AfterControlPlaneUpgradeRequest.WorkersUpgrades %s, want %s", request.WorkersUpgrades, wantRequest.WorkersUpgrades)
			}
		} else {
			return errors.Errorf("got an unexpected request of type %T", request)
		}
	}
	if request, ok := request.(*runtimehooksv1.AfterWorkersUpgradeRequest); ok {
		if wantRequest, ok := wantRequest.(*runtimehooksv1.AfterWorkersUpgradeRequest); ok && wantRequest != nil {
			if wantRequest.KubernetesVersion != request.KubernetesVersion {
				return errors.Errorf("unexpected AfterWorkersUpgradeRequest.Kubernetes version %s, want %s", request.KubernetesVersion, wantRequest.KubernetesVersion)
			}
			if !reflect.DeepEqual(wantRequest.ControlPlaneUpgrades, request.ControlPlaneUpgrades) {
				return errors.Errorf("unexpected AfterWorkersUpgradeRequest.ControlPlaneUpgrades %s, want %s", request.ControlPlaneUpgrades, wantRequest.ControlPlaneUpgrades)
			}
			if !reflect.DeepEqual(wantRequest.WorkersUpgrades, request.WorkersUpgrades) {
				return errors.Errorf("unexpected AfterWorkersUpgradeRequest.WorkersUpgrades %s, want %s", request.WorkersUpgrades, wantRequest.WorkersUpgrades)
			}
		} else {
			return errors.Errorf("got an unexpected request of type %T", request)
		}
	}
	return nil
}

func validateClusterParameter(originalCluster *clusterv1.Cluster) func(req runtimehooksv1.RequestObject) error {
	// return a func that allows to check if expected transformations are applied to the Cluster parameter which is
	// included in the payload for lifecycle hooks calls.
	return func(req runtimehooksv1.RequestObject) error {
		var cluster clusterv1beta1.Cluster
		switch req := req.(type) {
		case *runtimehooksv1.BeforeClusterUpgradeRequest:
			cluster = req.Cluster
		case *runtimehooksv1.BeforeControlPlaneUpgradeRequest:
			cluster = req.Cluster
		case *runtimehooksv1.AfterControlPlaneUpgradeRequest:
			cluster = req.Cluster
		case *runtimehooksv1.BeforeWorkersUpgradeRequest:
			cluster = req.Cluster
		case *runtimehooksv1.AfterWorkersUpgradeRequest:
			cluster = req.Cluster
		default:
			return errors.Errorf("unhandled request type %T", req)
		}

		// check if managed fields and well know annotations have been removed from the Cluster parameter included in the payload lifecycle hooks calls.
		if cluster.GetManagedFields() != nil {
			return errors.New("managedFields should have been cleaned up")
		}
		if _, ok := cluster.Annotations[corev1.LastAppliedConfigAnnotation]; ok {
			return errors.New("last-applied-configuration annotation should have been cleaned up")
		}
		if _, ok := cluster.Annotations[conversion.DataAnnotation]; ok {
			return errors.New("conversion annotation should have been cleaned up")
		}

		// check the Cluster parameter included in the payload lifecycle hooks calls has been properly converted from v1beta2 to v1beta1.
		// Note: to perform this check we convert the parameter back to v1beta2 and compare with the original cluster +/- expected transformations.
		v1beta2Cluster := &clusterv1.Cluster{}
		if err := cluster.ConvertTo(v1beta2Cluster); err != nil {
			return err
		}

		originalClusterCopy := originalCluster.DeepCopy()
		originalClusterCopy.SetManagedFields(nil)
		if originalClusterCopy.Annotations != nil {
			annotations := maps.Clone(cluster.Annotations)
			delete(annotations, corev1.LastAppliedConfigAnnotation)
			delete(annotations, conversion.DataAnnotation)
			originalClusterCopy.Annotations = annotations
		}

		// drop conditions, it is not possible to round trip without the data annotation.
		originalClusterCopy.Status.Conditions = nil

		if !apiequality.Semantic.DeepEqual(originalClusterCopy, v1beta2Cluster) {
			return errors.Errorf("call to extension is not passing the expected cluster object: %s", cmp.Diff(originalClusterCopy, v1beta2Cluster))
		}
		return nil
	}
}
