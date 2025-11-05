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

package machinedeployment

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conversion"
)

func TestComputeDesiredNewMS(t *testing.T) {
	t.Run("should compute Revision annotations for newMS, no oldMS", func(t *testing.T) {
		g := NewWithT(t)

		deployment := &clusterv1.MachineDeployment{}

		p := rolloutPlanner{
			md: deployment,
			// Add a dummy computeDesiredMS, that simply return an empty MS object.
			// Note: there is dedicate test to validate the actual computeDesiredMS func, it is ok to simplify the unit test here.
			overrideComputeDesiredMS: func(_ context.Context, _ *clusterv1.MachineDeployment, _ *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
				return &clusterv1.MachineSet{}, nil
			},
		}
		actualNewMS, err := p.computeDesiredNewMS(ctx, nil)
		g.Expect(err).ToNot(HaveOccurred())

		// annotations that we are intentionally not setting in this func are not there
		g.Expect(actualNewMS.Annotations).To(Equal(map[string]string{
			clusterv1.RevisionAnnotation:             "1",
			clusterv1.DisableMachineCreateAnnotation: "false",
		}))
	})
	t.Run("should update Revision annotations for newMS when required", func(t *testing.T) {
		g := NewWithT(t)

		deployment := &clusterv1.MachineDeployment{}
		currentMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "1",
				},
			},
		}
		oldMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "2",
				},
			},
		}

		p := rolloutPlanner{
			md: deployment,
			// Add a dummy computeDesiredMS, that simply pass through the currentNewMS.
			// Note: there is dedicate test to validate the actual computeDesiredMS func, it is ok to simplify the unit test here.
			overrideComputeDesiredMS: func(_ context.Context, _ *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
				return currentNewMS, nil
			},
			oldMSs: []*clusterv1.MachineSet{oldMS},
		}
		actualNewMS, err := p.computeDesiredNewMS(ctx, currentMS)
		g.Expect(err).ToNot(HaveOccurred())

		// annotations that we are intentionally not setting in this func are not there
		// Note: there is a dedicated test for ComputeRevisionAnnotations, so it is ok to have a minimal coverage here about revision management.
		g.Expect(actualNewMS.Annotations).To(Equal(map[string]string{
			clusterv1.RevisionAnnotation:                           "3",
			"machinedeployment.clusters.x-k8s.io/revision-history": "1",
			clusterv1.DisableMachineCreateAnnotation:               "false",
		}))
	})
	t.Run("should preserve Revision annotations for newMS when already up to date", func(t *testing.T) {
		g := NewWithT(t)

		deployment := &clusterv1.MachineDeployment{}
		currentMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation:                           "3",
					"machinedeployment.clusters.x-k8s.io/revision-history": "1",
				},
			},
		}
		oldMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "2",
				},
			},
		}

		p := rolloutPlanner{
			md: deployment,
			// Add a dummy computeDesiredMS, that simply pass through the currentNewMS.
			// Note: there is dedicate test to validate the actual computeDesiredMS func, it is ok to simplify the unit test here.
			overrideComputeDesiredMS: func(_ context.Context, _ *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
				return currentNewMS, nil
			},
			oldMSs: []*clusterv1.MachineSet{oldMS},
		}
		actualNewMS, err := p.computeDesiredNewMS(ctx, currentMS)
		g.Expect(err).ToNot(HaveOccurred())

		// annotations that we are intentionally not setting in this func are not there
		// Note: there is a dedicated test for ComputeRevisionAnnotations, so it is ok to have a minimal coverage here about revision management.
		g.Expect(actualNewMS.Annotations).To(Equal(map[string]string{
			clusterv1.RevisionAnnotation:                           "3",
			"machinedeployment.clusters.x-k8s.io/revision-history": "1",
			clusterv1.DisableMachineCreateAnnotation:               "false",
		}))
	})
}

func TestComputeDesiredOldMS(t *testing.T) {
	t.Run("should carry over Revision annotations from oldMS", func(t *testing.T) {
		g := NewWithT(t)
		const revision = "4"
		const revisionHistory = "1,3"

		deployment := &clusterv1.MachineDeployment{}
		currentMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation:                           revision,
					"machinedeployment.clusters.x-k8s.io/revision-history": revisionHistory,
				},
			},
		}

		p := rolloutPlanner{
			md: deployment,
			// Add a dummy computeDesiredMS, that simply pass through the currentNewMS.
			// Note: there is dedicate test to validate the actual computeDesiredMS func, it is ok to simplify the unit test here.
			overrideComputeDesiredMS: func(_ context.Context, _ *clusterv1.MachineDeployment, currentOldMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
				return currentOldMS, nil
			},
		}
		actualOldMS, err := p.computeDesiredOldMS(ctx, currentMS)
		g.Expect(err).ToNot(HaveOccurred())

		// annotations that we are intentionally not setting in this func are not there
		g.Expect(actualOldMS.Annotations).To(Equal(map[string]string{
			clusterv1.RevisionAnnotation:                           revision,
			"machinedeployment.clusters.x-k8s.io/revision-history": revisionHistory,
			clusterv1.DisableMachineCreateAnnotation:               "false",
		}))
	})
	t.Run("should disable creation of machines on oldMS when rollout strategy is OnDelete", func(t *testing.T) {
		g := NewWithT(t)
		const revision = "4"
		const revisionHistory = "1,3"

		deployment := &clusterv1.MachineDeployment{
			Spec: clusterv1.MachineDeploymentSpec{
				Rollout: clusterv1.MachineDeploymentRolloutSpec{Strategy: clusterv1.MachineDeploymentRolloutStrategy{Type: clusterv1.OnDeleteMachineDeploymentStrategyType}},
			},
		}
		currentMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation:                           revision,
					"machinedeployment.clusters.x-k8s.io/revision-history": revisionHistory,
				},
			},
		}

		p := rolloutPlanner{
			md: deployment,
			// Add a dummy computeDesiredMS, that simply pass through the currentNewMS.
			// Note: there is dedicate test to validate the actual computeDesiredMS func, it is ok to simplify the unit test here.
			overrideComputeDesiredMS: func(_ context.Context, _ *clusterv1.MachineDeployment, currentOldMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
				return currentOldMS, nil
			},
		}
		actualOldMS, err := p.computeDesiredOldMS(ctx, currentMS)
		g.Expect(err).ToNot(HaveOccurred())

		// annotations that we are intentionally not setting in this func are not there
		g.Expect(actualOldMS.Annotations).To(Equal(map[string]string{
			clusterv1.RevisionAnnotation:                           revision,
			"machinedeployment.clusters.x-k8s.io/revision-history": revisionHistory,
			clusterv1.DisableMachineCreateAnnotation:               "true",
		}))
	})
}

func TestComputeDesiredMS(t *testing.T) {
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
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
			Labels: map[string]string{
				// labels that must be propagated to MS.
				"machine-label1": "machine-value1",
			},
			Annotations: map[string]string{
				// annotations that must be propagated to MS.
				"top-level-annotation": "top-level-annotation-value",
			},
		},
		Spec: clusterv1.MachineSetSpec{
			// Info that we do expect to be copied from the MD.
			ClusterName: deployment.Spec.ClusterName,
			Deletion: clusterv1.MachineSetDeletionSpec{
				Order: deployment.Spec.Deletion.Order,
			},
			Selector:      deployment.Spec.Selector,
			Template:      *deployment.Spec.Template.DeepCopy(),
			MachineNaming: deployment.Spec.MachineNaming,
		},
	}

	t.Run("should compute a new MachineSet when current MS is nil", func(t *testing.T) {
		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		// Replicas should always be set to zero on newMS.
		expectedMS.Spec.Replicas = ptr.To[int32](0)

		g := NewWithT(t)
		actualMS, err := computeDesiredMS(ctx, deployment, nil)
		g.Expect(err).ToNot(HaveOccurred())
		assertDesiredMS(g, deployment, actualMS, expectedMS)
	})

	t.Run("should compute the updated MachineSet when current MS is not nil", func(t *testing.T) {
		uid := apirand.String(5)
		name := "foo"
		finalizers := []string{"pre-existing-finalizer"}
		replicas := ptr.To(int32(1))
		uniqueLabelValue := "123"
		currentMS := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         "default",
				CreationTimestamp: metav1.Now(),
				Labels: map[string]string{
					// labels that must be carried over
					clusterv1.MachineDeploymentUniqueLabel: uniqueLabelValue,
					// unknown labels from current MS should not be considered for desired state (known labels are inferred from MD).
					"foo": "bar",
				},
				Annotations: map[string]string{
					// unknown annotations from current MS should not be considered for desired state (known annotations are inferred from MD).
					"foo": "bar",
				},
				// value that must be preserved
				UID:        types.UID(uid),
				Name:       name,
				Finalizers: finalizers,
			},
			Spec: clusterv1.MachineSetSpec{
				// value that must be preserved
				Replicas: replicas,
				// Info that we do expect to be copied from the MD (set to another value to make sure it is overridden).
				Deletion: clusterv1.MachineSetDeletionSpec{
					Order: clusterv1.OldestMachineSetDeletionOrder,
				},
				MachineNaming: clusterv1.MachineNamingSpec{
					Template: "foo",
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "v1"},
				},
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels:      map[string]string{"foo": "machine-value1"},
						Annotations: map[string]string{"foo": "machine-value1"},
					},
					Spec: clusterv1.MachineSpec{
						Version:         "foo",
						MinReadySeconds: ptr.To[int32](5),
						ReadinessGates:  []clusterv1.MachineReadinessGate{{ConditionType: "bar"}},
						Deletion: clusterv1.MachineDeletionSpec{
							NodeDrainTimeoutSeconds:        nil,
							NodeVolumeDetachTimeoutSeconds: nil,
							NodeDeletionTimeoutSeconds:     nil,
						},
					},
				},
			},
		}

		expectedMS := skeletonMSBasedOnMD.DeepCopy()
		// Fields that are expected to be carried over from oldMS.
		expectedMS.ObjectMeta.Name = name
		expectedMS.Labels[clusterv1.MachineDeploymentUniqueLabel] = uniqueLabelValue
		expectedMS.ObjectMeta.UID = types.UID(uid)
		expectedMS.ObjectMeta.Finalizers = finalizers
		expectedMS.Spec.Replicas = replicas
		expectedMS.Spec.Template = *currentMS.Spec.Template.DeepCopy()
		// Fields that must be taken from the MD
		expectedMS.Spec.Deletion.Order = deployment.Spec.Deletion.Order
		expectedMS.Spec.MachineNaming = deployment.Spec.MachineNaming
		expectedMS.Spec.Template.Labels = mdutil.CloneAndAddLabel(deployment.Spec.Template.Labels, clusterv1.MachineDeploymentUniqueLabel, uniqueLabelValue)
		expectedMS.Spec.Template.Annotations = cloneStringMap(deployment.Spec.Template.Annotations)
		expectedMS.Spec.Template.Spec.MinReadySeconds = deployment.Spec.Template.Spec.MinReadySeconds
		expectedMS.Spec.Template.Spec.ReadinessGates = deployment.Spec.Template.Spec.ReadinessGates
		expectedMS.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds
		expectedMS.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds
		expectedMS.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds

		g := NewWithT(t)
		actualMS, err := computeDesiredMS(ctx, deployment, currentMS)
		g.Expect(err).ToNot(HaveOccurred())
		assertDesiredMS(g, deployment, actualMS, expectedMS)
	})
}

func assertDesiredMS(g *WithT, md *clusterv1.MachineDeployment, actualMS *clusterv1.MachineSet, expectedMS *clusterv1.MachineSet) {
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

	// Check CreationTimestamp
	g.Expect(actualMS.CreationTimestamp.IsZero()).Should(BeFalse())

	// Check Ownership
	g.Expect(util.IsControlledBy(actualMS, md, clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind())).Should(BeTrue())

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
	// annotations that must not be propagated from MD have been removed.
	g.Expect(actualMS.Annotations).ShouldNot(HaveKey(corev1.LastAppliedConfigAnnotation))
	g.Expect(actualMS.Annotations).ShouldNot(HaveKey(conversion.DataAnnotation))

	// annotations that must be derived from MD have been set.
	g.Expect(actualMS.Annotations).Should(HaveKeyWithValue(clusterv1.DesiredReplicasAnnotation, fmt.Sprintf("%d", *md.Spec.Replicas)))
	g.Expect(actualMS.Annotations).Should(HaveKeyWithValue(clusterv1.MaxReplicasAnnotation, fmt.Sprintf("%d", *(md.Spec.Replicas)+mdutil.MaxSurge(*md))))

	// annotations that we are intentionally not setting in this func are not there
	g.Expect(actualMS.Annotations).ShouldNot(HaveKey(clusterv1.RevisionAnnotation))
	g.Expect(actualMS.Annotations).ShouldNot(HaveKey("machinedeployment.clusters.x-k8s.io/revision-history"))
	g.Expect(actualMS.Annotations).ShouldNot(HaveKey(clusterv1.DisableMachineCreateAnnotation))

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

// machineControllerMutator fakes a small part of the Machine controller, just what is required for the rollout to progress.
func machineControllerMutator(log *fileLogger, m *clusterv1.Machine, scope *rolloutScope) {
	if m.DeletionTimestamp.IsZero() {
		return
	}

	log.Logf("[M controller] - %s finalizer removed", m.Name)
	ms := m.OwnerReferences[0].Name
	machinesSetMachines := []*clusterv1.Machine{}
	for _, mx := range scope.machineSetMachines[ms] {
		if mx.Name == m.Name {
			continue
		}
		machinesSetMachines = append(machinesSetMachines, mx)
	}
	scope.machineSetMachines[ms] = machinesSetMachines
}

// machineSetControllerMutator fakes a small part of the MachineSet controller, just what is required for the rollout to progress.
func machineSetControllerMutator(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope) error {
	// The prod code for the MachineSet controller performs in order triggerInPlaceUpdate and then syncReplicas and then updateStatus.
	// This func mimics the same code structure, with the addition of the following operation that is implemented here for convenience.

	// In order to simulate an in-place update being completed, remove the UpdateInProgressAnnotation from machines after
	// pendingAcknowledgeMove is gone in a previous reconcile.
	// Note: ideally this should be implemented in the fake Machine controller, but current implementation is
	// considered an acceptable trade-off because it provides a signal about in-place update completed, without
	// the need of adding the complexity of machine controller reconcile to the sequence tests/to the golden files
	// for the RollingUpdate strategy.
	replicasEndingInPlaceUpdate := sets.Set[string]{}
	for _, m := range scope.machineSetMachines[ms.Name] {
		if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; ok {
			continue
		}
		if _, ok := m.Annotations[clusterv1.UpdateInProgressAnnotation]; ok {
			delete(m.Annotations, clusterv1.UpdateInProgressAnnotation)
			replicasEndingInPlaceUpdate.Insert(m.Name)
		}
	}
	if replicasEndingInPlaceUpdate.Len() > 0 {
		log.Logf("[MS controller] - Replicas %s completed in place update", sortAndJoin(replicasEndingInPlaceUpdate.UnsortedList()))
	}

	// Starting from here, the code must mirror the implementation of the Reconcile func in the MachineSet controller to ensure the reliability of the tests.
	// NOTE: For consistency with the prod code, triggerInPlaceUpdate and then syncReplicas should not rely on ms.Status,
	// because ms.status is computed only at later stage by defer updateStatus.
	// (the code should rely on the list of machines instead).

	defer func() {
		machineSetControllerMutatorUpdateStatus(ms, scope)
	}()

	machineSetControllerMutatorTriggerInPlaceUpdate(ms, scope)
	return machineSetControllerMutatorSyncReplicas(log, ms, scope)
}

func machineSetControllerMutatorTriggerInPlaceUpdate(ms *clusterv1.MachineSet, scope *rolloutScope) {
	// Code below this line is a subset of the code from MachineSet controller's triggerInPlaceUpdate func, e.g.
	// it does not complete the move operation (what is implemented in moveMachine is enough for this test),
	// nor it sets the pendingHook annotation.

	// If the existing machine is pending acknowledge from the MD controller after a move operation,
	// wait until if it possible to drop the PendingAcknowledgeMove annotation.
	for _, m := range scope.machineSetMachines[ms.Name] {
		if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; ok {
			// Check if this MachineSet is still accepting machines moved from other MachineSets.
			if sourceMSs, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
				// Get the list of machines acknowledged by the MD controller.
				acknowledgedMoveReplicas := sets.Set[string]{}
				if replicaNames, ok := ms.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok && replicaNames != "" {
					acknowledgedMoveReplicas.Insert(strings.Split(replicaNames, ",")...)
				}

				// If the current machine is in not yet in the list, it is not possible to trigger in-place yet.
				if !acknowledgedMoveReplicas.Has(m.Name) {
					continue
				}

				// If the current machine is in the list, drop the annotation.
				delete(m.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
			} else {
				// If this MachineSet is not accepting anymore machines from other MS (e.g. because of MD spec changes),
				// then drop the PendingAcknowledgeMove annotation; this machine will be treated as any other machine and either
				// deleted or moved to another MS after completing the in-place upgrade.
				delete(m.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
			}
		}
	}
}

func machineSetControllerMutatorSyncReplicas(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope) error {
	// Code below this line is a subset of the code from MachineSet controller's "syncReplicas" func.

	diff := len(scope.machineSetMachines[ms.Name]) - int(ptr.Deref(ms.Spec.Replicas, 0))
	switch {
	case diff < 0:
		// If there are not enough Machines, create missing Machines unless Machine creation is disabled.
		machinesToAdd := -diff
		if ms.Annotations != nil {
			if value, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok && value == "true" {
				return nil
			}
		}
		machineSetControllerMutatorCreateMachines(log, ms, scope, machinesToAdd)

	case diff > 0:
		// if too many replicas, delete or move exceeding machines.

		// If the MachineSet is accepting replicas from other MachineSets (and thus this is the newMS controlled by a MD),
		// detect if there are replicas still pending AcknowledgedMove.
		// Note: replicas still pending AcknowledgeMove should not be counted when computing the numbers of machines to delete, because those machines are not included in ms.Spec.Replicas yet.
		// Without this check, the following logic would try to align the number of replicas to "an incomplete" ms.Spec.Replicas and as a consequence wrongly delete replicas that should be preserved.
		notAcknowledgeMoveReplicas := sets.Set[string]{}
		if sourceMSs, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
			for _, m := range scope.machineSetMachines[ms.Name] {
				if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; !ok {
					continue
				}
				notAcknowledgeMoveReplicas.Insert(m.Name)
			}
		}
		if notAcknowledgeMoveReplicas.Len() > 0 {
			log.Logf("[MS controller] - Replicas %s moved from an old MachineSet still pending acknowledge from machine deployment %s", sortAndJoin(notAcknowledgeMoveReplicas.UnsortedList()), klog.KObj(scope.machineDeployment))
		}

		machinesToDeleteOrMove := int32(len(scope.machineSetMachines[ms.Name])-notAcknowledgeMoveReplicas.Len()) - ptr.Deref(ms.Spec.Replicas, 0)
		if machinesToDeleteOrMove == 0 {
			return nil
		}

		// Move machines to the target MachineSet if the current MachineSet is instructed to do so.
		if targetMSName, ok := ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation]; ok && targetMSName != "" {
			// Note: The number of machines actually moved could be less than expected e.g. because some machine still updating in-place from a previous move.
			return machineSetControllerMutatorMoveMachines(log, ms, scope, targetMSName, machinesToDeleteOrMove)
		}

		// Otherwise the current MachineSet is not instructed to move machines to another MachineSet,
		// then delete all the exceeding machines.
		machineSetControllerMutatorDeleteMachines(log, ms, scope, machinesToDeleteOrMove)
		return nil
	}
	return nil
}

func machineSetControllerMutatorCreateMachines(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope, machinesToAdd int) {
	// Note: this is a simplified version of the code in the createMachines func from the MachineSet controller, e.g. no preflight checks,
	// no/lighter logging, no handling for infraMachine & BootstrapConfig, no event generation, no wait for cache up to date.
	// Note: In the code below, new machines are created with a predictable name, so it is easier to write test case and validate rollout sequences.
	// e.g. if the cluster is initialized with m1, m2, m3, new machines will be m4, m5, m6

	machinesAdded := []string{}
	for range machinesToAdd {
		machineName := fmt.Sprintf("m%d", scope.GetNextMachineUID())
		scope.machineSetMachines[ms.Name] = append(scope.machineSetMachines[ms.Name],
			&clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: machineName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: clusterv1.GroupVersion.String(),
							Kind:       "MachineSet",
							Name:       ms.Name,
							Controller: ptr.To(true),
						},
					},
				},
				Spec: *ms.Spec.Template.Spec.DeepCopy(),
			},
		)
		machinesAdded = append(machinesAdded, machineName)
	}

	// Sort machines of the target MS to ensure consistent reporting during tests.
	sortMachineSetMachinesByName(scope.machineSetMachines[ms.Name])

	log.Logf("[MS controller] - %s scale up to %d/%[2]d replicas (%s created)", ms.Name, ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesAdded, ","))
}

func machineSetControllerMutatorMoveMachines(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope, targetMSName string, machinesToMove int32) error {
	// Note: this is a simplified version of the code in the moveMachines func from the MachineSet controller, e.g. no pluggable move order,
	// no update of machine labels, no/lighter logging. Also please note that from the sake of this test, there is no split between start move an
	// completeMove (what is implemented below is enough to fake the entire move operation).
	// Note: in the test code exceeding machines are moved in predictable order, so it is easier to write test case and validate rollout sequences.
	// e.g. if a ms has m1,m2,m3 created in this order, m1 will be deleted first, then m2 and finally m3.

	var targetMS *clusterv1.MachineSet
	for _, ms2 := range scope.machineSets {
		if ms2.Name == targetMSName {
			targetMS = ms2
			break
		}
	}
	if targetMS == nil {
		return errors.Errorf("[MS controller] - PANIC! %s is set to send replicas to %s, which does not exists", ms.Name, targetMSName)
	}

	validSourceMSs := targetMS.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]
	sourcesSet := sets.Set[string]{}
	sourcesSet.Insert(strings.Split(validSourceMSs, ",")...)
	if !sourcesSet.Has(ms.Name) {
		return errors.Errorf("[MS controller] - PANIC! %s is set to send replicas to %s, but %[2]s only accepts machines from %s", ms.Name, targetMS.Name, validSourceMSs)
	}

	// Sort machines to ensure stable results of the move operation during tests.
	sortMachineSetMachinesByName(scope.machineSetMachines[ms.Name])

	// Always move all the machine that can be moved.
	// In case the target machine set will end up with more machines than its target replica number, it will take care of this.
	machinesMoved := []string{}
	machinesSetMachines := []*clusterv1.Machine{}
	for i, m := range scope.machineSetMachines[ms.Name] {
		// Make sure we are not moving machines deleting.
		if !m.DeletionTimestamp.IsZero() {
			machinesSetMachines = append(machinesSetMachines, m)
			continue
		}

		// Make sure we are not moving machines still updating in place (this includes also machines still pending AcknowledgeMove).
		if inplace.IsUpdateInProgress(m) {
			machinesSetMachines = append(machinesSetMachines, m)
			continue
		}

		if int32(len(machinesMoved)) >= machinesToMove {
			machinesSetMachines = append(machinesSetMachines, scope.machineSetMachines[ms.Name][i:]...)
			break
		}

		m := scope.machineSetMachines[ms.Name][i]
		if m.Annotations == nil {
			m.Annotations = map[string]string{}
		}
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "MachineSet",
				Name:       targetMS.Name,
				Controller: ptr.To(true),
			},
		}
		m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation] = ""
		m.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
		scope.machineSetMachines[targetMS.Name] = append(scope.machineSetMachines[targetMS.Name], m)
		machinesMoved = append(machinesMoved, m.Name)
	}
	scope.machineSetMachines[ms.Name] = machinesSetMachines
	log.Logf("[MS controller] - %s scale down to %d/%d replicas (%s moved to %s)", ms.Name, len(scope.machineSetMachines[ms.Name]), ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesMoved, ","), targetMS.Name)

	// Sort machines of the target MS to ensure consistent reporting during tests.
	// Note: It is also required to sort machines for the targetMS because both ms and targetMS lists of machines are changed in this func.
	sortMachineSetMachinesByName(scope.machineSetMachines[ms.Name])
	sortMachineSetMachinesByName(scope.machineSetMachines[targetMS.Name])
	return nil
}

func machineSetControllerMutatorDeleteMachines(log *fileLogger, ms *clusterv1.MachineSet, scope *rolloutScope, machinesToDelete int32) {
	// This is a simplified version of the code in the deleteMachines func from the MachineSet controller, e.g.
	// the test code does not consider the criteria defined in ms.Spec.Deletion.Order.

	machinesDeleted := []string{}

	// Note: in the test code exceeding machines are deleted in predictable order, so it is easier to write test case and validate rollout sequences.
	// e.g. if a ms has m1,m2,m3 created in this order, m1 will be deleted first, then m2 and finally m3.
	// Note: In case the system has to delete some machine, and there are machines are still updating in place, those machine must be deleted first.
	//
	// This prevents the system to perform unnecessary in-place updates. e.g.
	// - In place rollout of MD with 3 Replicas, maxSurge 1, MaxUnavailable 0
	//   - First create m4 to create a buffer for doing in place
	//   - Move old machines (m1, m2, m3)
	// - Resulting new MS at this point has 4 replicas m1, m2, m3 (updating in place) and (m4).
	// - The system scales down MS, and the system does this getting rid of m3 - the last replica that started in place.
	machinesSetMachinesSortedByDeletePriority := sortMachineSetMachinesByDeletionPriorityAndName(scope.machineSetMachines[ms.Name])

	newMachinesSetMachines := []*clusterv1.Machine{}
	for i, m := range machinesSetMachinesSortedByDeletePriority {
		if int32(len(machinesDeleted)) >= machinesToDelete {
			newMachinesSetMachines = append(newMachinesSetMachines, machinesSetMachinesSortedByDeletePriority[i:]...)
			break
		}
		machinesDeleted = append(machinesDeleted, m.Name)
	}
	scope.machineSetMachines[ms.Name] = newMachinesSetMachines

	// Sort machines to ensure consistent reporting during tests.
	sortMachineSetMachinesByName(scope.machineSetMachines[ms.Name])

	log.Logf("[MS controller] - %s scale down to %d/%[2]d replicas (%s deleted)", ms.Name, ptr.Deref(ms.Spec.Replicas, 0), strings.Join(machinesDeleted, ","))
}

func machineSetControllerMutatorUpdateStatus(ms *clusterv1.MachineSet, scope *rolloutScope) {
	// This is a simplified version of the code in the updateStatus func from the MachineSet controller.
	// Note: the corresponding logic in the MS controller looks at the MachineAvailable condition to
	// determine availability. Here we are looking at the UpdateInProgress annotation, which is a
	// one of the info considered by the MachineAvailable condition.

	ms.Status.Replicas = ptr.To(int32(len(scope.machineSetMachines[ms.Name])))
	availableReplicas := int32(0)
	for _, m := range scope.machineSetMachines[ms.Name] {
		if _, ok := m.Annotations[clusterv1.UpdateInProgressAnnotation]; ok {
			continue
		}
		availableReplicas++
	}
	ms.Status.AvailableReplicas = ptr.To(availableReplicas)
}

type rolloutScope struct {
	machineDeployment  *clusterv1.MachineDeployment
	machineSets        []*clusterv1.MachineSet
	machineSetMachines map[string][]*clusterv1.Machine

	machineUID int32
}

func initCurrentRolloutScope(currentMachineNames []string, mdOptions ...machineDeploymentOption) (current *rolloutScope) {
	// create current state, with a MD with
	// - given MaxSurge, MaxUnavailable
	// - replica counters assuming all the machines are at stable state
	// - spec different from the MachineSets and Machines we are going to create down below (to simulate a change that triggers a rollout, but it is not yet started)
	mdReplicaCount := int32(len(currentMachineNames))
	current = &rolloutScope{
		machineDeployment: createMD("v2", mdReplicaCount, mdOptions...),
	}

	// Create current MS, with
	// - replica counters assuming all the machines are at stable state
	// - spec at stable state (rollout is not yet propagated to machines)
	ms := createMS("ms1", "v1", mdReplicaCount)
	current.machineSets = append(current.machineSets, ms)

	// Create current Machines, with
	// - spec at stable state (rollout is not yet propagated to machines)
	var totMachines int32
	currentMachines := []*clusterv1.Machine{}
	for _, machineSetMachineName := range currentMachineNames {
		totMachines++
		currentMachines = append(currentMachines, createM(machineSetMachineName, ms.Name, ms.Spec.Template.Spec.FailureDomain))
	}
	current.machineSetMachines = map[string][]*clusterv1.Machine{}
	current.machineSetMachines[ms.Name] = currentMachines
	current.machineUID = totMachines

	return current
}

func computeDesiredRolloutScope(current *rolloutScope, desiredMachineNames []string) (desired *rolloutScope) {
	var totMachineSets, totMachines int32
	totMachineSets = int32(len(current.machineSets))
	for _, msMachines := range current.machineSetMachines {
		totMachines += int32(len(msMachines))
	}

	// Create current state, with a MD equal to the one we started from because:
	// - spec was already changed in current to simulate a change that triggers a rollout
	// - desired replica counters are the same than current replica counters (we start with all the machines at stable state v1, we should end with all the machines at stable state v2)
	desired = &rolloutScope{
		machineDeployment: current.machineDeployment.DeepCopy(),
	}
	desired.machineDeployment.Status.Replicas = desired.machineDeployment.Spec.Replicas
	desired.machineDeployment.Status.AvailableReplicas = desired.machineDeployment.Spec.Replicas

	// Add current MS to desired state, but set replica counters to zero because all the machines must be moved to the new MS.
	// Note: one of the old MS could also be the NewMS, the MS that must become owner of all the desired machines.
	var newMS *clusterv1.MachineSet
	for _, currentMS := range current.machineSets {
		oldMS := currentMS.DeepCopy()
		oldMS.Spec.Replicas = ptr.To(int32(0))
		oldMS.Status.Replicas = ptr.To(int32(0))
		oldMS.Status.AvailableReplicas = ptr.To(int32(0))
		desired.machineSets = append(desired.machineSets, oldMS)

		if upToDate, _ := mdutil.MachineTemplateUpToDate(&oldMS.Spec.Template, &desired.machineDeployment.Spec.Template); upToDate {
			if newMS != nil {
				panic("there should be only one MachineSet with MachineTemplateUpToDate")
			}
			newMS = oldMS
		}
	}

	// Add or update the new MS to desired state, with
	// - the new spec from the MD
	// - replica counters assuming all the replicas must be here at the end of the rollout.
	if newMS != nil {
		newMS.Spec.Replicas = desired.machineDeployment.Spec.Replicas
		newMS.Status.Replicas = desired.machineDeployment.Status.Replicas
		newMS.Status.AvailableReplicas = desired.machineDeployment.Status.AvailableReplicas
	} else {
		totMachineSets++
		newMS = createMS(fmt.Sprintf("ms%d", totMachineSets), desired.machineDeployment.Spec.Template.Spec.FailureDomain, *desired.machineDeployment.Spec.Replicas)
		desired.machineSets = append(desired.machineSets, newMS)
	}

	// Add a desired machines to desired state, with
	// - the new spec from the MD (steady state)
	desiredMachines := []*clusterv1.Machine{}
	for _, machineSetMachineName := range desiredMachineNames {
		totMachines++
		desiredMachines = append(desiredMachines, createM(machineSetMachineName, newMS.Name, newMS.Spec.Template.Spec.FailureDomain))
	}
	desired.machineSetMachines = map[string][]*clusterv1.Machine{}
	desired.machineSetMachines[newMS.Name] = desiredMachines
	return desired
}

// GetNextMachineUID provides a predictable UID for machines.
func (r *rolloutScope) GetNextMachineUID() int32 {
	r.machineUID++
	return r.machineUID
}

func (r *rolloutScope) Clone() *rolloutScope {
	if r == nil {
		return nil
	}

	c := &rolloutScope{
		machineDeployment:  r.machineDeployment.DeepCopy(),
		machineSetMachines: map[string][]*clusterv1.Machine{},
		machineUID:         r.machineUID,
	}
	for _, ms := range r.machineSets {
		c.machineSets = append(c.machineSets, ms.DeepCopy())
	}
	for ms, machines := range r.machineSetMachines {
		cmachines := make([]*clusterv1.Machine, 0, len(machines))
		for _, m := range machines {
			cmachines = append(cmachines, m.DeepCopy())
		}
		c.machineSetMachines[ms] = cmachines
	}
	return c
}

func (r rolloutScope) String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%s, %d/%d replicas\n", r.machineDeployment.Name, ptr.Deref(r.machineDeployment.Status.Replicas, 0), ptr.Deref(r.machineDeployment.Spec.Replicas, 0)))

	sort.Slice(r.machineSets, func(i, j int) bool { return r.machineSets[i].Name < r.machineSets[j].Name })
	for _, ms := range r.machineSets {
		sb.WriteString(fmt.Sprintf("- %s, %s\n", ms.Name, msLog(ms, r.machineSetMachines[ms.Name])))
	}
	return sb.String()
}

func msLog(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) string {
	sb := strings.Builder{}
	machineNames := []string{}
	acknowledgedMoveMachines := sets.Set[string]{}
	if replicaNames, ok := ms.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok && replicaNames != "" {
		acknowledgedMoveMachines.Insert(strings.Split(replicaNames, ",")...)
	}
	for _, m := range machines {
		name := m.Name
		if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; ok && !acknowledgedMoveMachines.Has(name) {
			name += "🟠"
		}
		if _, ok := m.Annotations[clusterv1.UpdateInProgressAnnotation]; ok {
			name += "🟡"
		}
		machineNames = append(machineNames, name)
	}
	sb.WriteString(strings.Join(machineNames, ","))
	if moveTo, ok := ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation]; ok {
		sb.WriteString(fmt.Sprintf(" => %s", moveTo))
	}
	if moveFrom, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok {
		sb.WriteString(fmt.Sprintf(" <= %s", moveFrom))
	}
	msLog := fmt.Sprintf("%d/%d replicas (%s)", ptr.Deref(ms.Status.Replicas, 0), ptr.Deref(ms.Spec.Replicas, 0), sb.String())
	return msLog
}

func (r rolloutScope) machines() []*clusterv1.Machine {
	machines := []*clusterv1.Machine{}
	for _, ms := range r.machineSets {
		machines = append(machines, r.machineSetMachines[ms.Name]...)
	}
	return machines
}

func (r *rolloutScope) Equal(s *rolloutScope) bool {
	return machineDeploymentIsEqual(r.machineDeployment, s.machineDeployment) && machineSetsAreEqual(r.machineSets, s.machineSets) && machineSetMachinesAreEqual(r.machineSetMachines, s.machineSetMachines)
}

func machineDeploymentIsEqual(a, b *clusterv1.MachineDeployment) bool {
	if upToDate, _ := mdutil.MachineTemplateUpToDate(&a.Spec.Template, &b.Spec.Template); !upToDate ||
		ptr.Deref(a.Spec.Replicas, 0) != ptr.Deref(b.Spec.Replicas, 0) ||
		ptr.Deref(a.Status.Replicas, 0) != ptr.Deref(b.Status.Replicas, 0) ||
		ptr.Deref(a.Status.AvailableReplicas, 0) != ptr.Deref(b.Status.AvailableReplicas, 0) {
		return false
	}
	return true
}

func machineSetsAreEqual(a, b []*clusterv1.MachineSet) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]*clusterv1.MachineSet)
	for i := range a {
		aMap[a[i].Name] = a[i]
	}

	for i := range b {
		desiredMS := b[i]
		currentMS, ok := aMap[desiredMS.Name]
		if !ok {
			return false
		}
		if upToDate, _ := mdutil.MachineTemplateUpToDate(&desiredMS.Spec.Template, &currentMS.Spec.Template); !upToDate ||
			ptr.Deref(desiredMS.Spec.Replicas, 0) != ptr.Deref(currentMS.Spec.Replicas, 0) ||
			ptr.Deref(desiredMS.Status.Replicas, 0) != ptr.Deref(currentMS.Status.Replicas, 0) ||
			ptr.Deref(desiredMS.Status.AvailableReplicas, 0) != ptr.Deref(currentMS.Status.AvailableReplicas, 0) {
			return false
		}
	}
	return true
}

func machineSetMachinesAreEqual(a, b map[string][]*clusterv1.Machine) bool {
	for ms, aMachines := range a {
		bMachines, ok := b[ms]
		if !ok {
			if len(aMachines) > 0 {
				return false
			}
			continue
		}

		if len(aMachines) != len(bMachines) {
			return false
		}

		for i := range aMachines {
			if aMachines[i].Name != bMachines[i].Name {
				return false
			}
			if len(aMachines[i].OwnerReferences) != 1 || len(bMachines[i].OwnerReferences) != 1 || aMachines[i].OwnerReferences[0].Name != bMachines[i].OwnerReferences[0].Name {
				return false
			}
		}
	}
	return true
}

type UniqueRand struct {
	rng       *rand.Rand
	generated map[int]bool // keeps track of random numbers already generated.
	max       int          // max number to be generated
}

func (u *UniqueRand) Int() int {
	if u.Done() {
		return -1
	}
	for {
		i := u.rng.Intn(u.max)
		if !u.generated[i] {
			u.generated[i] = true
			return i
		}
	}
}

func (u *UniqueRand) Done() bool {
	return len(u.generated) >= u.max
}

func (u *UniqueRand) Forget(n int) {
	delete(u.generated, n)
}

type fileLogger struct {
	t *testing.T

	testCase              string
	fileName              string
	testCaseStringBuilder strings.Builder
	writeGoldenFile       bool
}

func newFileLogger(t *testing.T, name, fileName string) *fileLogger {
	t.Helper()

	l := &fileLogger{t: t, testCaseStringBuilder: strings.Builder{}}
	l.testCaseStringBuilder.WriteString(fmt.Sprintf("## %s\n\n", name))
	l.testCase = name
	l.fileName = fileName
	return l
}

func (l *fileLogger) Logf(format string, args ...interface{}) {
	l.t.Logf(format, args...)

	// this codes takes a log line that has been formatted for t.Logf and change it
	// so it will look nice in the files. e.g. adds indentation to all lines except the fist one, which by convention starts with [.
	s := strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
	sb := &strings.Builder{}
	if strings.Contains(s, "\n") {
		lines := strings.Split(s, "\n")
		for _, line := range lines {
			indent := "  "
			if strings.HasPrefix(line, "[") {
				indent = ""
			}
			sb.WriteString(indent + line + "\n")
		}
	} else {
		sb.WriteString(s + "\n")
	}
	l.testCaseStringBuilder.WriteString(sb.String())
}

func (l *fileLogger) WriteLogAndCompareWithGoldenFile() (string, string, error) {
	if err := os.WriteFile(fmt.Sprintf("%s.test.log", l.fileName), []byte(l.testCaseStringBuilder.String()), 0600); err != nil {
		return "", "", err
	}
	if l.writeGoldenFile {
		if err := os.WriteFile(fmt.Sprintf("%s.test.log.golden", l.fileName), []byte(l.testCaseStringBuilder.String()), 0600); err != nil {
			return "", "", err
		}
	}

	currentBytes, _ := os.ReadFile(fmt.Sprintf("%s.test.log", l.fileName))
	current := string(currentBytes)

	goldenBytes, _ := os.ReadFile(fmt.Sprintf("%s.test.log.golden", l.fileName))
	golden := string(goldenBytes)

	return current, golden, nil
}

func sortMachineSetMachinesByName(machines []*clusterv1.Machine) {
	sort.Slice(machines, func(i, j int) bool {
		iNameIndex, _ := strconv.Atoi(strings.TrimPrefix(machines[i].Name, "m"))
		jNameIndex, _ := strconv.Atoi(strings.TrimPrefix(machines[j].Name, "m"))
		return iNameIndex < jNameIndex
	})
}

func sortMachineSetMachinesByDeletionPriorityAndName(machines []*clusterv1.Machine) []*clusterv1.Machine {
	machinesSetMachinesSortedByDeletePriority := slices.Clone(machines)

	// Note: machines updating in place must be deleted first.
	// in case of ties:
	// - if both machines are updating in place, delete first the machine with the highest machine NameIndex (e.g. between m3 and m4, pick m4, aka the last machine being moved)
	// - if both machines are not updating in place, delete first the machine with the lowest machine NameIndex (e.g. between m3 and m4, pick m3)
	sort.Slice(machinesSetMachinesSortedByDeletePriority, func(i, j int) bool {
		iPriority := 100
		if inplace.IsUpdateInProgress(machinesSetMachinesSortedByDeletePriority[i]) {
			iPriority = 1
		}
		jPriority := 100
		if inplace.IsUpdateInProgress(machinesSetMachinesSortedByDeletePriority[j]) {
			jPriority = 1
		}
		if iPriority == jPriority {
			iNameIndex, _ := strconv.Atoi(strings.TrimPrefix(machinesSetMachinesSortedByDeletePriority[i].Name, "m"))
			jNameIndex, _ := strconv.Atoi(strings.TrimPrefix(machinesSetMachinesSortedByDeletePriority[j].Name, "m"))
			if iPriority == 1 {
				return iNameIndex > jNameIndex
			}
			return iNameIndex < jNameIndex
		}
		return iPriority < jPriority
	})
	return machinesSetMachinesSortedByDeletePriority
}

// default task order ensure the controllers are run in a consistent and predictable way: md, ms1, ms2 and so on.
func defaultTaskOrder(taskCount int) []int {
	taskOrder := []int{}
	for t := range taskCount {
		taskOrder = append(taskOrder, t)
	}
	return taskOrder
}

func randomTaskOrder(taskCount int, rng *rand.Rand) []int {
	u := &UniqueRand{
		rng:       rng,
		generated: map[int]bool{},
		max:       taskCount,
	}
	taskOrder := []int{}
	for !u.Done() {
		n := u.Int()
		if rng.Intn(10) < 3 { // skip a step in the 30% of cases
			continue
		}
		taskOrder = append(taskOrder, n)
		if r := rng.Intn(10); r < 3 { // repeat a step in the 30% of cases
			u.Forget(n)
		}
	}
	return taskOrder
}

type machineDeploymentOption func(md *clusterv1.MachineDeployment)

func withRollingUpdateStrategy(maxSurge, maxUnavailable int32) func(md *clusterv1.MachineDeployment) {
	return func(md *clusterv1.MachineDeployment) {
		md.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
				MaxSurge:       ptr.To(intstr.FromInt32(maxSurge)),
				MaxUnavailable: ptr.To(intstr.FromInt32(maxUnavailable)),
			},
		}
	}
}

func withOnDeleteStrategy() func(md *clusterv1.MachineDeployment) {
	return func(md *clusterv1.MachineDeployment) {
		md.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
		}
	}
}

func createMD(failureDomain string, replicas int32, options ...machineDeploymentOption) *clusterv1.MachineDeployment {
	md := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "md"},
		Spec: clusterv1.MachineDeploymentSpec{
			// Note: using failureDomain as a template field to determine upToDate
			Template: clusterv1.MachineTemplateSpec{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			Replicas: &replicas,
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          &replicas,
			AvailableReplicas: &replicas,
		},
	}
	for _, opt := range options {
		opt(md)
	}
	return md
}

func withStatusAvailableReplicas(r int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.AvailableReplicas = ptr.To(r)
	}
}

func withStatusUpToDateReplicas(r int32) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		ms.Status.UpToDateReplicas = ptr.To(r)
	}
}

func withMSAnnotation(name, value string) fakeMachineSetOption {
	return func(ms *clusterv1.MachineSet) {
		if ms.Annotations == nil {
			ms.Annotations = map[string]string{}
		}
		ms.Annotations[name] = value
	}
}

func createMS(name, failureDomain string, replicas int32, opts ...fakeMachineSetOption) *clusterv1.MachineSet {
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.MachineSetSpec{
			// Note: using failureDomain as a template field to determine upToDate
			Template: clusterv1.MachineTemplateSpec{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			Replicas: ptr.To(replicas),
		},
		Status: clusterv1.MachineSetStatus{
			Replicas:          ptr.To(replicas),
			AvailableReplicas: ptr.To(replicas),
		},
	}
	for _, opt := range opts {
		opt(ms)
	}
	return ms
}

func withMAnnotation(name, value string) fakeMachinesOption {
	return func(m *clusterv1.Machine) {
		if m.Annotations == nil {
			m.Annotations = map[string]string{}
		}
		m.Annotations[name] = value
	}
}

func createM(name, ownedByMS, failureDomain string, opts ...fakeMachinesOption) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       ownedByMS,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			// Note: using failureDomain as a template field to determine upToDate
			FailureDomain: failureDomain,
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
