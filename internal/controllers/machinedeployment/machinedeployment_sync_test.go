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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestCalculateStatus(t *testing.T) {
	msStatusError := capierrors.MachineSetStatusError("some failure")

	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     *clusterv1.MachineDeployment
		expectedStatus clusterv1.MachineDeploymentStatus
	}{
		"all machines are running": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  2,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  2,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   2,
				UnavailableReplicas: 0,
				Phase:               "Running",
			},
		},
		"scaling up": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  1,
					ReadyReplicas:      1,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  1,
					ReadyReplicas:      1,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       1,
				AvailableReplicas:   1,
				UnavailableReplicas: 1,
				Phase:               "ScalingUp",
			},
		},
		"scaling down": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  3,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  3,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   3,
				UnavailableReplicas: 0,
				Phase:               "ScalingDown",
			},
		},
		"MachineSet failed": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  0,
					ReadyReplicas:      0,
					Replicas:           2,
					ObservedGeneration: 1,
					FailureReason:      &msStatusError,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  0,
					ReadyReplicas:      0,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       0,
				AvailableReplicas:   0,
				UnavailableReplicas: 2,
				Phase:               "Failed",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			actualStatus := calculateStatus(test.machineSets, test.newMachineSet, test.deployment)
			g.Expect(actualStatus).To(Equal(test.expectedStatus))
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
					Replicas: pointer.Int32(2),
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
					Replicas: pointer.Int32(2),
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32(2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(0),
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32(2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(4),
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32(2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32(2),
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

			err := r.scaleMachineSet(context.Background(), tc.machineSet, tc.newScale, tc.machineDeployment)
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

func newTestMachineDeployment(pds *int32, replicas, statusReplicas, updatedReplicas, availableReplicas int32, conditions clusterv1.Conditions) *clusterv1.MachineDeployment {
	d := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ProgressDeadlineSeconds: pds,
			Replicas:                &replicas,
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: intOrStrPtr(0),
					MaxSurge:       intOrStrPtr(1),
					DeletePolicy:   pointer.String("Oldest"),
				},
			},
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          statusReplicas,
			UpdatedReplicas:   updatedReplicas,
			AvailableReplicas: availableReplicas,
			Conditions:        conditions,
		},
	}
	return d
}

// helper to create MS with given availableReplicas.
func newTestMachinesetWithReplicas(name string, specReplicas, statusReplicas, availableReplicas int32) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{},
			Namespace:         metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: pointer.Int32(specReplicas),
		},
		Status: clusterv1.MachineSetStatus{
			AvailableReplicas: availableReplicas,
			Replicas:          statusReplicas,
		},
	}
}

func TestSyncDeploymentStatus(t *testing.T) {
	pds := int32(60)
	tests := []struct {
		name               string
		d                  *clusterv1.MachineDeployment
		oldMachineSets     []*clusterv1.MachineSet
		newMachineSet      *clusterv1.MachineSet
		expectedConditions []*clusterv1.Condition
	}{
		{
			name:           "Deployment not available: MachineDeploymentAvailableCondition should exist and be false",
			d:              newTestMachineDeployment(&pds, 3, 2, 2, 2, clusterv1.Conditions{}),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 2, 2),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineDeploymentAvailableCondition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityWarning,
					Reason:   clusterv1.WaitingForAvailableMachinesReason,
				},
			},
		},
		{
			name:           "Deployment Available: MachineDeploymentAvailableCondition should exist and be true",
			d:              newTestMachineDeployment(&pds, 3, 3, 3, 3, clusterv1.Conditions{}),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 3, 3),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:   clusterv1.MachineDeploymentAvailableCondition,
					Status: corev1.ConditionTrue,
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
	duration5s := &metav1.Duration{Duration: 5 * time.Second}
	duration10s := &metav1.Duration{Duration: 10 * time.Second}

	infraRef := corev1.ObjectReference{
		Kind:       "GenericInfrastructureMachineTemplate",
		Name:       "infra-template-1",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
	}
	bootstrapRef := corev1.ObjectReference{
		Kind:       "GenericBootstrapConfigTemplate",
		Name:       "bootstrap-template-1",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
	}

	deployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "md1",
			Annotations: map[string]string{"top-level-annotation": "top-level-annotation-value"},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName:     "test-cluster",
			Replicas:        pointer.Int32(3),
			MinReadySeconds: pointer.Int32(10),
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxSurge:       intOrStrPtr(1),
					DeletePolicy:   pointer.String("Random"),
					MaxUnavailable: intOrStrPtr(0),
				},
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
					Version:           pointer.String("v1.25.3"),
					InfrastructureRef: infraRef,
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &bootstrapRef,
					},
					NodeDrainTimeout:        duration10s,
					NodeVolumeDetachTimeout: duration10s,
					NodeDeletionTimeout:     duration10s,
				},
			},
		},
	}

	skeletonMS := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Labels:      map[string]string{"machine-label1": "machine-value1"},
			Annotations: map[string]string{"top-level-annotation": "top-level-annotation-value"},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName:     "test-cluster",
			Replicas:        pointer.Int32(3),
			MinReadySeconds: 10,
			DeletePolicy:    "Random",
			Selector:        metav1.LabelSelector{MatchLabels: map[string]string{"k1": "v1"}},
			Template:        *deployment.Spec.Template.DeepCopy(),
		},
	}

	// Creating a new MS
	expectedNewMS := skeletonMS.DeepCopy()

	// Creating a new MS when old MSs exist
	oldMSWhenCreatingNewMS := skeletonMS.DeepCopy()
	oldMSWhenCreatingNewMS.Spec.Replicas = pointer.Int32(2)
	expectedNewMSRolling := skeletonMS.DeepCopy()
	expectedNewMSRolling.Spec.Replicas = pointer.Int32(2) // 4 (maxsurge+replicas) - 2 (replicas of old ms) = 2

	uniqueID := apirand.String(5)

	// Updating an existing MS, not old MSs
	existingMS := skeletonMS.DeepCopy()
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
	existingMS.Spec.Template.Spec.NodeDrainTimeout = duration5s
	existingMS.Spec.Template.Spec.NodeDeletionTimeout = duration5s
	existingMS.Spec.Template.Spec.NodeVolumeDetachTimeout = duration5s
	existingMS.Spec.DeletePolicy = "Newest"
	existingMS.Spec.MinReadySeconds = 0

	expectedUpdatedMS := skeletonMS.DeepCopy()
	expectedUpdatedMS.UID = existingMSUID
	expectedUpdatedMS.Name = deployment.Name + "-" + uniqueID
	expectedUpdatedMS.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
	expectedUpdatedMS.Spec.Template.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID

	// Updating an existing MS, old MSs exist
	oldMSWhenUpdatingExistingMS := skeletonMS.DeepCopy()
	oldMSWhenUpdatingExistingMS.Spec.Replicas = pointer.Int32(4)

	expectedUpdatedMSRolling := skeletonMS.DeepCopy()
	expectedUpdatedMSRolling.UID = existingMSUID
	expectedUpdatedMSRolling.Name = deployment.Name + "-" + uniqueID
	expectedUpdatedMSRolling.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID
	expectedUpdatedMSRolling.Spec.Template.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueID

	test := []struct {
		name       string
		deployment *clusterv1.MachineDeployment
		existingMS *clusterv1.MachineSet
		oldMSs     []*clusterv1.MachineSet
		wantErr    bool
		expectedMS *clusterv1.MachineSet
	}{
		{
			name:       "Computing a new MachineSet - no old MSs",
			deployment: deployment,
			existingMS: nil,
			oldMSs:     nil,
			wantErr:    false,
			expectedMS: expectedNewMS,
		},
		{
			name:       "Computing a new MachineSet - old MSs exist",
			deployment: deployment,
			existingMS: nil,
			oldMSs:     []*clusterv1.MachineSet{oldMSWhenCreatingNewMS},
			wantErr:    false,
			expectedMS: expectedNewMSRolling,
		},
		{
			name:       "Updating an existing MachineSet - no old MSs",
			deployment: deployment,
			existingMS: existingMS,
			oldMSs:     nil,
			wantErr:    false,
			expectedMS: expectedUpdatedMS,
		},
		{
			name:       "Updating an existing MachineSet - old MSs exist",
			deployment: deployment,
			existingMS: existingMS,
			oldMSs:     []*clusterv1.MachineSet{oldMSWhenUpdatingExistingMS},
			wantErr:    false,
			expectedMS: expectedUpdatedMSRolling,
		},
	}

	log := klogr.New()
	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &Reconciler{}
			computedMS, err := r.computeDesiredMachineSet(tt.deployment, tt.existingMS, tt.oldMSs, log)
			if tt.wantErr {
				g.Expect(err).NotTo(BeNil())
			} else {
				g.Expect(err).Should(BeNil())
				assertMachineSet(g, computedMS, tt.expectedMS)
			}
		})
	}
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
	// If the expected MS is a new MachineSet then make sure that the labels also has the unique identifier key
	if expectedMS.Name == "" {
		g.Expect(actualMS.Labels).Should(HaveKey(clusterv1.MachineDeploymentUniqueLabel))
		g.Expect(actualMS.Spec.Template.Labels).Should(HaveKey(clusterv1.MachineDeploymentUniqueLabel))
	}

	// Check Annotations
	// Note: More nuanced validation of the Revision annotation calculations are done when testing `ComputeMachineSetAnnotations`.
	for k, v := range expectedMS.Annotations {
		g.Expect(actualMS.Annotations).Should(HaveKeyWithValue(k, v))
	}
	for k, v := range expectedMS.Spec.Template.Annotations {
		g.Expect(actualMS.Spec.Template.Annotations).Should(HaveKeyWithValue(k, v))
	}

	// Check MinReadySeconds
	g.Expect(actualMS.Spec.MinReadySeconds).Should(Equal(expectedMS.Spec.MinReadySeconds))

	// Check DeletePolicy
	g.Expect(actualMS.Spec.DeletePolicy).Should(Equal(expectedMS.Spec.DeletePolicy))

	// Check MachineTemplateSpec
	g.Expect(actualMS.Spec.Template.Spec).Should(Equal(expectedMS.Spec.Template.Spec))
}

// asserts the conditions set on the Getter object.
// TODO: replace this with util.condition.MatchConditions (or a new matcher in controller runtime komega).
func assertConditions(t *testing.T, from conditions.Getter, conditions ...*clusterv1.Condition) {
	t.Helper()

	for _, condition := range conditions {
		assertCondition(t, from, condition)
	}
}

// asserts whether a condition of type is set on the Getter object
// when the condition is true, asserting the reason/severity/message
// for the condition are avoided.
func assertCondition(t *testing.T, from conditions.Getter, condition *clusterv1.Condition) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(conditions.Has(from, condition.Type)).To(BeTrue())

	if condition.Status == corev1.ConditionTrue {
		conditions.IsTrue(from, condition.Type)
	} else {
		conditionToBeAsserted := conditions.Get(from, condition.Type)
		g.Expect(conditionToBeAsserted.Status).To(Equal(condition.Status))
		g.Expect(conditionToBeAsserted.Severity).To(Equal(condition.Severity))
		g.Expect(conditionToBeAsserted.Reason).To(Equal(condition.Reason))
		if condition.Message != "" {
			g.Expect(conditionToBeAsserted.Message).To(Equal(condition.Message))
		}
	}
}
