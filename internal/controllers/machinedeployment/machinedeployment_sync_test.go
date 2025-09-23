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

package machinedeployment

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func TestCalculateV1Beta1Status(t *testing.T) {
	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     *clusterv1.MachineDeployment
		expectedStatus clusterv1.MachineDeploymentStatus
	}{
		"all machines are running": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 2,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 2,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       2,
						AvailableReplicas:   2,
						UnavailableReplicas: 0,
					},
				},
			},
		},
		"scaling up": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 1,
							ReadyReplicas:     1,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 1,
							ReadyReplicas:     1,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       1,
						AvailableReplicas:   1,
						UnavailableReplicas: 1,
					},
				},
			},
		},
		"scaling down": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 3,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 3,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       2,
						AvailableReplicas:   3,
						UnavailableReplicas: 0,
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			calculateV1Beta1Status(test.machineSets, test.newMachineSet, test.deployment)
			g.Expect(test.deployment.Status).To(BeComparableTo(test.expectedStatus))
		})
	}
}

func TestScaleMachineSet(t *testing.T) {
	testCases := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machineSet        *clusterv1.MachineSet
		newScale          int32
		error             error
	}{
		{
			name: "It fails when new MachineSet has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			error: errors.Errorf("spec.replicas for MachineSet foo/bar is nil, this is unexpected"),
		},
		{
			name: "It fails when new MachineDeployment has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			error: errors.Errorf("spec.replicas for MachineDeployment foo/bar is nil, this is unexpected"),
		},
		{
			name: "Scale up",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
				},
			},
			newScale: 2,
		},
		{
			name: "Scale down",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](4),
				},
			},
			newScale: 2,
		},
		{
			name: "Same replicas does not scale",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			newScale: 2,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			resources := []client.Object{
				tc.machineDeployment,
				tc.machineSet,
			}

			r := &Reconciler{
				Client:   fake.NewClientBuilder().WithObjects(resources...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			err := r.scaleMachineSet(t.Context(), tc.machineSet, tc.newScale, tc.machineDeployment)
			if tc.error != nil {
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			freshMachineSet := &clusterv1.MachineSet{}
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(tc.machineSet), freshMachineSet)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(*freshMachineSet.Spec.Replicas).To(BeEquivalentTo(tc.newScale))

			expectedMachineSetAnnotations := map[string]string{
				clusterv1.DesiredReplicasAnnotation: fmt.Sprintf("%d", *tc.machineDeployment.Spec.Replicas),
				clusterv1.MaxReplicasAnnotation:     fmt.Sprintf("%d", (*tc.machineDeployment.Spec.Replicas)+mdutil.MaxSurge(*tc.machineDeployment)),
			}
			g.Expect(freshMachineSet.GetAnnotations()).To(BeEquivalentTo(expectedMachineSetAnnotations))
		})
	}
}

func newTestMachineDeployment(replicas, statusReplicas, upToDateReplicas, availableReplicas int32) *clusterv1.MachineDeployment {
	d := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: &replicas,
			Rollout: clusterv1.MachineDeploymentRolloutSpec{
				Strategy: clusterv1.MachineDeploymentRolloutStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
						MaxUnavailable: intOrStrPtr(0),
						MaxSurge:       intOrStrPtr(1),
					},
				},
			},
			Deletion: clusterv1.MachineDeploymentDeletionSpec{
				Order: clusterv1.OldestMachineSetDeletionOrder,
			},
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          ptr.To[int32](statusReplicas),
			UpToDateReplicas:  ptr.To[int32](upToDateReplicas),
			AvailableReplicas: ptr.To[int32](availableReplicas),
		},
	}
	return d
}

// helper to create MS with given availableReplicas.
func newTestMachinesetWithReplicas(name string, specReplicas, statusReplicas, availableReplicas int32, v1Beta1Conditions clusterv1.Conditions) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{},
			Namespace:         metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](specReplicas),
		},
		Status: clusterv1.MachineSetStatus{
			Replicas:          ptr.To(statusReplicas),
			AvailableReplicas: ptr.To[int32](availableReplicas),
			Deprecated: &clusterv1.MachineSetDeprecatedStatus{
				V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
					Conditions: v1Beta1Conditions,
				},
			},
		},
	}
}

func TestSyncDeploymentStatus(t *testing.T) {
	tests := []struct {
		name               string
		d                  *clusterv1.MachineDeployment
		oldMachineSets     []*clusterv1.MachineSet
		newMachineSet      *clusterv1.MachineSet
		expectedConditions []*clusterv1.Condition
	}{
		{
			name:           "Deployment not available: MachineDeploymentAvailableCondition should exist and be false",
			d:              newTestMachineDeployment(3, 2, 2, 2),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 2, 2, nil),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineDeploymentAvailableV1Beta1Condition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityWarning,
					Reason:   clusterv1.WaitingForAvailableMachinesV1Beta1Reason,
				},
			},
		},
		{
			name:           "Deployment Available: MachineDeploymentAvailableCondition should exist and be true",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 3, 3, nil),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:   clusterv1.MachineDeploymentAvailableV1Beta1Condition,
					Status: corev1.ConditionTrue,
				},
			},
		},
		{
			name:           "MachineSet exist: MachineSetReadyCondition should exist and mirror MachineSet Ready condition",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet: newTestMachinesetWithReplicas("foo", 3, 3, 3, clusterv1.Conditions{
				{
					Type:    clusterv1.ReadyV1Beta1Condition,
					Status:  corev1.ConditionFalse,
					Reason:  "TestErrorResaon",
					Message: "test error messsage",
				},
			}),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:    clusterv1.MachineSetReadyV1Beta1Condition,
					Status:  corev1.ConditionFalse,
					Reason:  "TestErrorResaon",
					Message: "test error messsage",
				},
			},
		},
		{
			name:           "MachineSet doesn't exist: MachineSetReadyCondition should exist and be false",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  nil,
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineSetReadyV1Beta1Condition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityInfo,
					Reason:   clusterv1.WaitingForMachineSetFallbackV1Beta1Reason,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &Reconciler{
				Client:   fake.NewClientBuilder().Build(),
				recorder: record.NewFakeRecorder(32),
			}
			allMachineSets := append(test.oldMachineSets, test.newMachineSet)
			err := r.syncDeploymentStatus(allMachineSets, test.newMachineSet, test.d)
			g.Expect(err).ToNot(HaveOccurred())
			assertConditions(t, test.d, test.expectedConditions...)
		})
	}
}

func TestComputeDesiredMachineSet(t *testing.T) {
	duration5s := ptr.To(int32(5))
	duration10s := ptr.To(int32(10))
	namingTemplateKey := "test"

	infraRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericInfrastructureMachineTemplate",
		Name:     "infra-template-1",
		APIGroup: clusterv1.GroupVersionInfrastructure.Group,
	}
	bootstrapRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericBootstrapConfigTemplate",
		Name:     "bootstrap-template-1",
		APIGroup: clusterv1.GroupVersionBootstrap.Group,
	}

	deployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "md1",
			Annotations: map[string]string{"top-level-annotation": "top-level-annotation-value"},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "test-cluster",
			Replicas:    ptr.To[int32](3),
			Rollout: clusterv1.MachineDeploymentRolloutSpec{
				Strategy: clusterv1.MachineDeploymentRolloutStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
						MaxSurge:       intOrStrPtr(1),
						MaxUnavailable: intOrStrPtr(0),
					},
				},
			},
			Deletion: clusterv1.MachineDeploymentDeletionSpec{
				Order: clusterv1.RandomMachineSetDeletionOrder,
			},
			MachineNaming: clusterv1.MachineNamingSpec{
				Template: "{{ .machineSet.name }}" + namingTemplateKey + "-{{ .random }}",
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"k1": "v1"},
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels:      map[string]string{"machine-label1": "machine-value1"},
					Annotations: map[string]string{"machine-annotation1": "machine-value1"},
				},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.25.3",
					InfrastructureRef: infraRef,
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: bootstrapRef,
					},
					MinReadySeconds: ptr.To[int32](3),
					ReadinessGates:  []clusterv1.MachineReadinessGate{{ConditionType: "foo"}},
					Deletion: clusterv1.MachineDeletionSpec{
						NodeDrainTimeoutSeconds:        duration10s,
						NodeVolumeDetachTimeoutSeconds: duration10s,
						NodeDeletionTimeoutSeconds:     duration10s,
					},
				},
			},
		},
	}

	skeletonMSBasedOnMD := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Labels:      map[string]string{"machine-label1": "machine-value1"},
			Annotations: map[string]string{"top-level-annotation": "top-level-annotation-value"},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: "test-cluster",
			Replicas:    ptr.To[int32](3),
			Deletion: clusterv1.MachineSetDeletionSpec{
				Order: clusterv1.RandomMachineSetDeletionOrder,
			},
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{"k1": "v1"}},
			Template: *deployment.Spec.Template.DeepCopy(),
			MachineNaming: clusterv1.MachineNamingSpec{
				Template: "{{ .machineSet.name }}" + namingTemplateKey + "-{{ .random }}",
			},
		},
	}

	t.Run("should compute a new MachineSet when no old MachineSets exist", func(t *testing.T) {
		expectedMS := skeletonMSBasedOnMD.DeepCopy()

		g := NewWithT(t)
		actualMS, err := (&Reconciler{}).computeDesiredMachineSet(ctx, deployment, nil, nil)
		g.Expect(err).ToNot(HaveOccurred())
		assertMachineSet(g, actualMS, expectedMS)
	})

	t.Run("should compute a new MachineSet when old MachineSets exist", func(t *testing.T) {
		oldMS := skeletonMSBasedOnMD.DeepCopy()
		oldMS.Spec.Replicas = ptr.To[int32](2)

		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		expectedMS.Spec.Replicas = ptr.To[int32](2) // 4 (maxsurge+replicas) - 2 (replicas of old ms) = 2

		g := NewWithT(t)
		actualMS, err := (&Reconciler{}).computeDesiredMachineSet(ctx, deployment, nil, []*clusterv1.MachineSet{oldMS})
		g.Expect(err).ToNot(HaveOccurred())
		assertMachineSet(g, actualMS, expectedMS)
	})

	t.Run("should compute the updated MachineSet when no old MachineSets exists", func(t *testing.T) {
		uniqueID := apirand.String(5)
		existingMS := skeletonMSBasedOnMD.DeepCopy()
		// computeDesiredMachineSet should retain the UID, name and the "machine-template-hash" label value
		// of the existing machine.
		// Other fields like labels, annotations, node timeout, etc are expected to change.
		existingMSUID := types.UID("abc-123-uid")
		existingMS.UID = existingMSUID
		existingMS.Name = deployment.Name + "-" + uniqueID
		existingMS.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-1":                           "ms-value-1",
		}
		existingMS.Annotations = nil
		// Pre-existing finalizer should be preserved.
		existingMS.Finalizers = []string{"pre-existing-finalizer"}
		existingMS.Spec.Template.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-2":                           "ms-value-2",
		}
		existingMS.Spec.Template.Annotations = nil
		existingMS.Spec.Template.Spec.ReadinessGates = []clusterv1.MachineReadinessGate{{ConditionType: "bar"}}
		existingMS.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration5s
		existingMS.Spec.Deletion.Order = clusterv1.NewestMachineSetDeletionOrder
		existingMS.Spec.Template.Spec.MinReadySeconds = ptr.To[int32](0)

		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		expectedMS.UID = existingMSUID
		expectedMS.Name = deployment.Name + "-" + uniqueID
		expectedMS.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
		// Pre-existing finalizer should be preserved.
		expectedMS.Finalizers = []string{"pre-existing-finalizer"}

		expectedMS.Spec.Template.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID

		g := NewWithT(t)
		actualMS, err := (&Reconciler{}).computeDesiredMachineSet(ctx, deployment, existingMS, nil)
		g.Expect(err).ToNot(HaveOccurred())
		assertMachineSet(g, actualMS, expectedMS)
	})

	t.Run("should compute the updated MachineSet when old MachineSets exist", func(t *testing.T) {
		uniqueID := apirand.String(5)
		existingMS := skeletonMSBasedOnMD.DeepCopy()
		existingMSUID := types.UID("abc-123-uid")
		existingMS.UID = existingMSUID
		existingMS.Name = deployment.Name + "-" + uniqueID
		existingMS.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-1":                           "ms-value-1",
		}
		existingMS.Annotations = nil
		// Pre-existing finalizer should be preserved.
		existingMS.Finalizers = []string{"pre-existing-finalizer"}
		existingMS.Spec.Template.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-2":                           "ms-value-2",
		}
		existingMS.Spec.Template.Annotations = nil
		existingMS.Spec.Template.Spec.ReadinessGates = []clusterv1.MachineReadinessGate{{ConditionType: "bar"}}
		existingMS.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration5s
		existingMS.Spec.Deletion.Order = clusterv1.NewestMachineSetDeletionOrder
		existingMS.Spec.Template.Spec.MinReadySeconds = ptr.To[int32](0)

		oldMS := skeletonMSBasedOnMD.DeepCopy()
		oldMS.Spec.Replicas = ptr.To[int32](2)

		// Note: computeDesiredMachineSet does not modify the replicas on the updated MachineSet.
		// Therefore, even though we have the old machineset with replicas 2 the updatedMS does not
		// get modified replicas (2 = 4(maxsuge+spec.replica) - 2(oldMS replicas)).
		// Nb. The final replicas of the MachineSet are calculated elsewhere.
		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		expectedMS.UID = existingMSUID
		expectedMS.Name = deployment.Name + "-" + uniqueID
		expectedMS.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
		// Pre-existing finalizer should be preserved.
		expectedMS.Finalizers = []string{"pre-existing-finalizer"}
		expectedMS.Spec.Template.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID

		g := NewWithT(t)
		actualMS, err := (&Reconciler{}).computeDesiredMachineSet(ctx, deployment, existingMS, []*clusterv1.MachineSet{oldMS})
		g.Expect(err).ToNot(HaveOccurred())
		assertMachineSet(g, actualMS, expectedMS)
	})

	t.Run("should compute the updated MachineSet when no old MachineSets exists (", func(t *testing.T) {
		// Set rollout strategy to "OnDelete".
		deployment := deployment.DeepCopy()
		deployment.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type:          clusterv1.OnDeleteMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{},
		}

		uniqueID := apirand.String(5)
		existingMS := skeletonMSBasedOnMD.DeepCopy()
		// computeDesiredMachineSet should retain the UID, name and the "machine-template-hash" label value
		// of the existing machine.
		// Other fields like labels, annotations, node timeout, etc are expected to change.
		existingMSUID := types.UID("abc-123-uid")
		existingMS.UID = existingMSUID
		existingMS.Name = deployment.Name + "-" + uniqueID
		existingMS.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-1":                           "ms-value-1",
		}
		existingMS.Annotations = nil
		existingMS.Spec.Template.Labels = map[string]string{
			clusterv1.MachineDeploymentUniqueLabel: uniqueID,
			"ms-label-2":                           "ms-value-2",
		}
		existingMS.Spec.Template.Annotations = nil
		existingMS.Spec.Template.Spec.ReadinessGates = []clusterv1.MachineReadinessGate{{ConditionType: "bar"}}
		existingMS.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = duration5s
		existingMS.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration5s
		existingMS.Spec.Deletion.Order = clusterv1.NewestMachineSetDeletionOrder
		existingMS.Spec.Template.Spec.MinReadySeconds = ptr.To[int32](0)

		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		expectedMS.UID = existingMSUID
		expectedMS.Name = deployment.Name + "-" + uniqueID
		expectedMS.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
		expectedMS.Spec.Template.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
		expectedMS.Spec.Deletion.Order = deployment.Spec.Deletion.Order

		g := NewWithT(t)
		actualMS, err := (&Reconciler{}).computeDesiredMachineSet(ctx, deployment, existingMS, nil)
		g.Expect(err).ToNot(HaveOccurred())
		assertMachineSet(g, actualMS, expectedMS)
	})
}

func assertMachineSet(g *WithT, actualMS *clusterv1.MachineSet, expectedMS *clusterv1.MachineSet) {
	// check UID
	if expectedMS.UID != "" {
		g.Expect(actualMS.UID).Should(Equal(expectedMS.UID))
	}
	// Check Name
	if expectedMS.Name != "" {
		g.Expect(actualMS.Name).Should(Equal(expectedMS.Name))
	}
	// Check Namespace
	g.Expect(actualMS.Namespace).Should(Equal(expectedMS.Namespace))

	// Check finalizers
	g.Expect(actualMS.Finalizers).Should(Equal(expectedMS.Finalizers))

	// Check Replicas
	g.Expect(actualMS.Spec.Replicas).ShouldNot(BeNil())
	g.Expect(actualMS.Spec.Replicas).Should(HaveValue(Equal(*expectedMS.Spec.Replicas)))

	// Check ClusterName
	g.Expect(actualMS.Spec.ClusterName).Should(Equal(expectedMS.Spec.ClusterName))

	// Check Labels
	for k, v := range expectedMS.Labels {
		g.Expect(actualMS.Labels).Should(HaveKeyWithValue(k, v))
	}
	for k, v := range expectedMS.Spec.Template.Labels {
		g.Expect(actualMS.Spec.Template.Labels).Should(HaveKeyWithValue(k, v))
	}
	// Verify that the labels also has the unique identifier key.
	g.Expect(actualMS.Labels).Should(HaveKey(clusterv1.MachineDeploymentUniqueLabel))
	g.Expect(actualMS.Spec.Template.Labels).Should(HaveKey(clusterv1.MachineDeploymentUniqueLabel))

	// Check Annotations
	// Note: More nuanced validation of the Revision annotation calculations are done when testing `ComputeMachineSetAnnotations`.
	for k, v := range expectedMS.Annotations {
		g.Expect(actualMS.Annotations).Should(HaveKeyWithValue(k, v))
	}
	for k, v := range expectedMS.Spec.Template.Annotations {
		g.Expect(actualMS.Spec.Template.Annotations).Should(HaveKeyWithValue(k, v))
	}

	// Check MinReadySeconds
	g.Expect(actualMS.Spec.Template.Spec.MinReadySeconds).Should(Equal(expectedMS.Spec.Template.Spec.MinReadySeconds))

	// Check Order
	g.Expect(actualMS.Spec.Deletion.Order).Should(Equal(expectedMS.Spec.Deletion.Order))

	// Check MachineTemplateSpec
	g.Expect(actualMS.Spec.Template.Spec).Should(BeComparableTo(expectedMS.Spec.Template.Spec))

	// Check MachineNamingSpec
	g.Expect(actualMS.Spec.MachineNaming.Template).Should(BeComparableTo(expectedMS.Spec.MachineNaming.Template))
}

// asserts the conditions set on the Getter object.
// TODO: replace this with util.condition.MatchConditions (or a new matcher in controller runtime komega).
func assertConditions(t *testing.T, from v1beta1conditions.Getter, conditions ...*clusterv1.Condition) {
	t.Helper()

	for _, condition := range conditions {
		assertCondition(t, from, condition)
	}
}

// asserts whether a condition of type is set on the Getter object
// when the condition is true, asserting the reason/severity/message
// for the condition are avoided.
func assertCondition(t *testing.T, from v1beta1conditions.Getter, condition *clusterv1.Condition) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(v1beta1conditions.Has(from, condition.Type)).To(BeTrue())

	if condition.Status == corev1.ConditionTrue {
		v1beta1conditions.IsTrue(from, condition.Type)
	} else {
		conditionToBeAsserted := v1beta1conditions.Get(from, condition.Type)
		g.Expect(conditionToBeAsserted.Status).To(Equal(condition.Status))
		g.Expect(conditionToBeAsserted.Severity).To(Equal(condition.Severity))
		g.Expect(conditionToBeAsserted.Reason).To(Equal(condition.Reason))
		if condition.Message != "" {
			g.Expect(conditionToBeAsserted.Message).To(Equal(condition.Message))
		}
	}
}

func Test_computeNewMachineSetName(t *testing.T) {
	tests := []struct {
		base       string
		wantPrefix string
	}{
		{
			"a",
			"a",
		},
		{
			fmt.Sprintf("%058d", 0),
			fmt.Sprintf("%058d", 0),
		},
		{
			fmt.Sprintf("%059d", 0),
			fmt.Sprintf("%058d", 0),
		},
		{
			fmt.Sprintf("%0100d", 0),
			fmt.Sprintf("%058d", 0),
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("base=%q, wantPrefix=%q", tt.base, tt.wantPrefix), func(t *testing.T) {
			got, gotSuffix := computeNewMachineSetName(tt.base)
			gotPrefix := strings.TrimSuffix(got, gotSuffix)
			if gotPrefix != tt.wantPrefix {
				t.Errorf("computeNewMachineSetName() = (%v, %v) wantPrefix %v", got, gotSuffix, tt.wantPrefix)
			}
			if len(got) > maxNameLength {
				t.Errorf("expected %s to be of max length %d", got, maxNameLength)
			}
		})
	}
}
