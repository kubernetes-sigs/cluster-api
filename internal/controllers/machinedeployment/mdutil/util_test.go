/*
Copyright 2018 The Kubernetes Authors.

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

package mdutil

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conversion"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func newDControllerRef(md *clusterv1.MachineDeployment) *metav1.OwnerReference {
	isController := true
	return &metav1.OwnerReference{
		APIVersion: "clusters/v1alpha",
		Kind:       "MachineDeployment",
		Name:       md.GetName(),
		UID:        md.GetUID(),
		Controller: &isController,
	}
}

// generateMS creates a machine set, with the input deployment's template as its template.
func generateMS(md clusterv1.MachineDeployment) clusterv1.MachineSet {
	template := md.Spec.Template.DeepCopy()
	return clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:             randomUID(),
			Name:            names.SimpleNameGenerator.GenerateName("machineset"),
			Labels:          template.Labels,
			OwnerReferences: []metav1.OwnerReference{*newDControllerRef(&md)},
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: new(int32),
			Template: *template,
			Selector: metav1.LabelSelector{MatchLabels: template.Labels},
		},
	}
}

func randomUID() types.UID {
	return types.UID(strconv.FormatInt(rand.Int63(), 10)) //nolint:gosec
}

// generateDeployment creates a deployment, with the input image as its template.
func generateDeployment(image string) clusterv1.MachineDeployment {
	machineLabels := map[string]string{"name": image}
	return clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        image,
			Annotations: make(map[string]string),
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: ptr.To[int32](3),
			Selector: metav1.LabelSelector{MatchLabels: machineLabels},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: machineLabels,
				},
				Spec: clusterv1.MachineSpec{
					Deletion: clusterv1.MachineDeletionSpec{
						NodeDrainTimeoutSeconds: ptr.To(int32(10)),
					},
				},
			},
		},
	}
}

func TestMachineSetsByDecreasingReplicas(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(1 * time.Minute)
	msAReplicas1T0 := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: t0},
			Name:              "ms-a",
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](1),
		},
	}

	msAAReplicas3T0 := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: t0},
			Name:              "ms-aa",
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](3),
		},
	}

	msBReplicas1T0 := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: t0},
			Name:              "ms-b",
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](1),
		},
	}

	msAReplicas1T1 := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: t1},
			Name:              "ms-a",
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](1),
		},
	}

	tests := []struct {
		name        string
		machineSets []*clusterv1.MachineSet
		want        []*clusterv1.MachineSet
	}{
		{
			name:        "machine set with higher replicas should be lower in the list",
			machineSets: []*clusterv1.MachineSet{msAReplicas1T0, msAAReplicas3T0},
			want:        []*clusterv1.MachineSet{msAAReplicas3T0, msAReplicas1T0},
		},
		{
			name:        "MachineSet created earlier should be lower in the list if replicas are the same",
			machineSets: []*clusterv1.MachineSet{msAReplicas1T1, msAReplicas1T0},
			want:        []*clusterv1.MachineSet{msAReplicas1T0, msAReplicas1T1},
		},
		{
			name:        "MachineSet with lower name should be lower in the list if the replicas and creationTimestamp are same",
			machineSets: []*clusterv1.MachineSet{msBReplicas1T0, msAReplicas1T0},
			want:        []*clusterv1.MachineSet{msAReplicas1T0, msBReplicas1T0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// sort the machine sets and verify the sorted list
			g := NewWithT(t)
			sort.Sort(MachineSetsByDecreasingReplicas(tt.machineSets))
			g.Expect(tt.machineSets).To(BeComparableTo(tt.want))
		})
	}
}

func TestMachineTemplateUpToDate(t *testing.T) {
	machineTemplate := &clusterv1.MachineTemplateSpec{
		ObjectMeta: clusterv1.ObjectMeta{
			Labels:      map[string]string{"l1": "v1"},
			Annotations: map[string]string{"a1": "v1"},
		},
		Spec: clusterv1.MachineSpec{
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        ptr.To(int32(10)),
				NodeDeletionTimeoutSeconds:     ptr.To(int32(10)),
				NodeVolumeDetachTimeoutSeconds: ptr.To(int32(10)),
			},
			ClusterName:     "cluster1",
			Version:         "v1.25.0",
			FailureDomain:   "failure-domain1",
			MinReadySeconds: ptr.To[int32](10),
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Name:     "infra1",
				Kind:     "InfrastructureMachineTemplate",
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Name:     "bootstrap1",
					Kind:     "BootstrapConfigTemplate",
					APIGroup: clusterv1.GroupVersionBootstrap.Group,
				},
			},
			Taints: []clusterv1.MachineTaint{
				{Key: "taint-key", Value: "taint-value", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways},
			},
		},
	}

	machineTemplateEqual := machineTemplate.DeepCopy()

	machineTemplateWithEmptyLabels := machineTemplate.DeepCopy()
	machineTemplateWithEmptyLabels.Labels = map[string]string{}

	machineTemplateWithDifferentLabels := machineTemplate.DeepCopy()
	machineTemplateWithDifferentLabels.Labels = map[string]string{"l2": "v2"}

	machineTemplateWithEmptyAnnotations := machineTemplate.DeepCopy()
	machineTemplateWithEmptyAnnotations.Annotations = map[string]string{}

	machineTemplateWithDifferentAnnotations := machineTemplate.DeepCopy()
	machineTemplateWithDifferentAnnotations.Annotations = map[string]string{"a2": "v2"}

	machineTemplateWithDifferentInPlaceMutableSpecFields := machineTemplate.DeepCopy()
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.ReadinessGates = []clusterv1.MachineReadinessGate{{ConditionType: "foo"}}
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(20))
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.Deletion.NodeDeletionTimeoutSeconds = ptr.To(int32(20))
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = ptr.To(int32(20))
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.MinReadySeconds = ptr.To[int32](20)
	machineTemplateWithDifferentInPlaceMutableSpecFields.Spec.Taints = []clusterv1.MachineTaint{
		{Key: "taint-key", Value: "taint-value", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways},
		{Key: "other-key", Value: "other-value", Effect: corev1.TaintEffectNoExecute, Propagation: clusterv1.MachineTaintPropagationAlways},
	}

	machineTemplateWithDifferentClusterName := machineTemplate.DeepCopy()
	machineTemplateWithDifferentClusterName.Spec.ClusterName = "cluster2"

	machineTemplateWithDifferentVersion := machineTemplate.DeepCopy()
	machineTemplateWithDifferentVersion.Spec.Version = "v1.26.0"

	machineTemplateWithDifferentFailureDomain := machineTemplate.DeepCopy()
	machineTemplateWithDifferentFailureDomain.Spec.FailureDomain = "failure-domain2"

	machineTemplateWithDifferentInfraRef := machineTemplate.DeepCopy()
	machineTemplateWithDifferentInfraRef.Spec.InfrastructureRef.Name = "infra2"

	machineTemplateWithBootstrapDataSecret := machineTemplate.DeepCopy()
	machineTemplateWithBootstrapDataSecret.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{} // Overwriting ConfigRef to empty.
	machineTemplateWithBootstrapDataSecret.Spec.Bootstrap.DataSecretName = ptr.To("data-secret1")

	machineTemplateWithDifferentBootstrapDataSecret := machineTemplateWithBootstrapDataSecret.DeepCopy()
	machineTemplateWithDifferentBootstrapDataSecret.Spec.Bootstrap.DataSecretName = ptr.To("data-secret2")

	machineTemplateWithDifferentBootstrapConfigRef := machineTemplate.DeepCopy()
	machineTemplateWithDifferentBootstrapConfigRef.Spec.Bootstrap.ConfigRef.Name = "bootstrap2"

	tests := []struct {
		Name                       string
		current, desired           *clusterv1.MachineTemplateSpec
		expectedUpToDate           bool
		expectedLogMessages1       []string
		expectedLogMessages2       []string
		expectedConditionMessages1 []string
		expectedConditionMessages2 []string
	}{
		{
			Name: "Same spec",
			// Note: This test ensures that two MachineTemplates are equal even if the pointers differ.
			current:          machineTemplate,
			desired:          machineTemplateEqual,
			expectedUpToDate: true,
		},
		{
			Name:             "Same spec, except desired does not have labels",
			current:          machineTemplate,
			desired:          machineTemplateWithEmptyLabels,
			expectedUpToDate: true,
		},
		{
			Name:             "Same spec, except desired has different labels",
			current:          machineTemplate,
			desired:          machineTemplateWithDifferentLabels,
			expectedUpToDate: true,
		},
		{
			Name:             "Same spec, except desired does not have annotations",
			current:          machineTemplate,
			desired:          machineTemplateWithEmptyAnnotations,
			expectedUpToDate: true,
		},
		{
			Name:             "Same spec, except desired has different annotations",
			current:          machineTemplate,
			desired:          machineTemplateWithDifferentAnnotations,
			expectedUpToDate: true,
		},
		{
			Name:             "Spec changes, desired has different in-place mutable spec fields",
			current:          machineTemplate,
			desired:          machineTemplateWithDifferentInPlaceMutableSpecFields,
			expectedUpToDate: true,
		},
		{
			Name:                       "Spec changes, desired has different Version",
			current:                    machineTemplate,
			desired:                    machineTemplateWithDifferentVersion,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.version v1.25.0, v1.26.0 required"},
			expectedLogMessages2:       []string{"spec.version v1.26.0, v1.25.0 required"},
			expectedConditionMessages1: []string{"Version v1.25.0, v1.26.0 required"},
			expectedConditionMessages2: []string{"Version v1.26.0, v1.25.0 required"},
		},
		{
			Name:                       "Spec changes, desired has different FailureDomain",
			current:                    machineTemplate,
			desired:                    machineTemplateWithDifferentFailureDomain,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.failureDomain failure-domain1, failure-domain2 required"},
			expectedLogMessages2:       []string{"spec.failureDomain failure-domain2, failure-domain1 required"},
			expectedConditionMessages1: []string{"Failure domain failure-domain1, failure-domain2 required"},
			expectedConditionMessages2: []string{"Failure domain failure-domain2, failure-domain1 required"},
		},
		{
			Name:                       "Spec changes, desired has different InfrastructureRef",
			current:                    machineTemplate,
			desired:                    machineTemplateWithDifferentInfraRef,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.infrastructureRef InfrastructureMachineTemplate infra1, InfrastructureMachineTemplate infra2 required"},
			expectedLogMessages2:       []string{"spec.infrastructureRef InfrastructureMachineTemplate infra2, InfrastructureMachineTemplate infra1 required"},
			expectedConditionMessages1: []string{"InfrastructureMachine is not up-to-date"},
			expectedConditionMessages2: []string{"InfrastructureMachine is not up-to-date"},
		},
		{
			Name:                       "Spec changes, desired has different Bootstrap data secret",
			current:                    machineTemplateWithBootstrapDataSecret,
			desired:                    machineTemplateWithDifferentBootstrapDataSecret,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.bootstrap.dataSecretName data-secret1, data-secret2 required"},
			expectedLogMessages2:       []string{"spec.bootstrap.dataSecretName data-secret2, data-secret1 required"},
			expectedConditionMessages1: []string{"spec.bootstrap.dataSecretName data-secret1, data-secret2 required"},
			expectedConditionMessages2: []string{"spec.bootstrap.dataSecretName data-secret2, data-secret1 required"},
		},
		{
			Name:                       "Spec changes, desired has different Bootstrap.ConfigRef",
			current:                    machineTemplate,
			desired:                    machineTemplateWithDifferentBootstrapConfigRef,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.bootstrap.configRef BootstrapConfigTemplate bootstrap1, BootstrapConfigTemplate bootstrap2 required"},
			expectedLogMessages2:       []string{"spec.bootstrap.configRef BootstrapConfigTemplate bootstrap2, BootstrapConfigTemplate bootstrap1 required"},
			expectedConditionMessages1: []string{"BootstrapConfig is not up-to-date"},
			expectedConditionMessages2: []string{"BootstrapConfig is not up-to-date"},
		},
		{
			Name:                       "Spec changes, desired has data secret instead of Bootstrap.ConfigRef",
			current:                    machineTemplate,
			desired:                    machineTemplateWithBootstrapDataSecret,
			expectedUpToDate:           false,
			expectedLogMessages1:       []string{"spec.bootstrap.configRef BootstrapConfigTemplate bootstrap1,   required"},
			expectedLogMessages2:       []string{"spec.bootstrap.dataSecretName data-secret1, nil required"},
			expectedConditionMessages1: []string{"BootstrapConfig is not up-to-date"},
			expectedConditionMessages2: []string{"spec.bootstrap.dataSecretName data-secret1, nil required"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			g := NewWithT(t)

			runTest := func(t1, t2 *clusterv1.MachineTemplateSpec, expectedLogMessages, expectedConditionMessages []string) {
				// Run
				upToDate, upToDateResult := MachineTemplateUpToDate(t1, t2)
				g.Expect(upToDate).To(Equal(test.expectedUpToDate))
				if upToDate {
					g.Expect(upToDateResult).ToNot(BeNil())
					g.Expect(upToDateResult.EligibleForInPlaceUpdate).To(BeFalse())
					g.Expect(upToDateResult.LogMessages).To(BeEmpty())
					g.Expect(upToDateResult.ConditionMessages).To(BeEmpty())
				} else {
					g.Expect(upToDateResult).ToNot(BeNil())
					g.Expect(upToDateResult.EligibleForInPlaceUpdate).To(BeTrue())
					g.Expect(upToDateResult.LogMessages).To(Equal(expectedLogMessages))
					g.Expect(upToDateResult.ConditionMessages).To(Equal(expectedConditionMessages))
				}
				g.Expect(t1.Labels).NotTo(BeNil())
				g.Expect(t2.Labels).NotTo(BeNil())
			}

			runTest(test.current, test.desired, test.expectedLogMessages1, test.expectedConditionMessages1)
			// Test the same case in reverse order
			runTest(test.desired, test.current, test.expectedLogMessages2, test.expectedConditionMessages2)
		})
	}
}

func TestFindNewAndOldMachineSets(t *testing.T) {
	threeBeforeRolloutAfter := metav1.Now()
	twoBeforeRolloutAfter := metav1.NewTime(threeBeforeRolloutAfter.Add(time.Minute))
	oneBeforeRolloutAfter := metav1.NewTime(twoBeforeRolloutAfter.Add(time.Minute))
	rolloutAfter := metav1.NewTime(oneBeforeRolloutAfter.Add(time.Minute))
	oneAfterRolloutAfter := metav1.NewTime(rolloutAfter.Add(time.Minute))
	twoAfterRolloutAfter := metav1.NewTime(oneAfterRolloutAfter.Add(time.Minute))

	deployment := generateDeployment("nginx")
	deployment.Spec.Template.Spec.InfrastructureRef.Kind = "InfrastructureMachineTemplate"
	deployment.Spec.Template.Spec.InfrastructureRef.Name = "new-infra-ref"

	deploymentWithRolloutAfter := deployment.DeepCopy()
	deploymentWithRolloutAfter.Spec.Rollout.After = rolloutAfter

	matchingMS := generateMS(deployment)

	matchingMSHigherReplicas := generateMS(deployment)
	matchingMSHigherReplicas.Spec.Replicas = ptr.To[int32](2)

	matchingMSDiffersInPlaceMutableFields := generateMS(deployment)
	matchingMSDiffersInPlaceMutableFields.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(20))
	matchingMSDiffersInPlaceMutableFields.Spec.Template.Spec.Taints = []clusterv1.MachineTaint{
		{Key: "taint-key", Value: "taint-value", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways},
	}

	oldMS := generateMS(deployment)
	oldMS.Spec.Template.Spec.InfrastructureRef.Name = "old-infra-ref"

	oldMSCreatedThreeBeforeRolloutAfter := *oldMS.DeepCopy()
	oldMSCreatedThreeBeforeRolloutAfter.CreationTimestamp = threeBeforeRolloutAfter

	msCreatedThreeBeforeRolloutAfter := generateMS(deployment)
	msCreatedThreeBeforeRolloutAfter.CreationTimestamp = threeBeforeRolloutAfter

	msCreatedTwoBeforeRolloutAfter := generateMS(deployment)
	msCreatedTwoBeforeRolloutAfter.CreationTimestamp = twoBeforeRolloutAfter

	msCreatedAfterRolloutAfter := generateMS(deployment)
	msCreatedAfterRolloutAfter.CreationTimestamp = oneAfterRolloutAfter

	msCreatedExactlyInRolloutAfter := generateMS(deployment)
	msCreatedExactlyInRolloutAfter.CreationTimestamp = rolloutAfter

	tests := []struct {
		Name                    string
		deployment              clusterv1.MachineDeployment
		msList                  []*clusterv1.MachineSet
		reconciliationTime      metav1.Time
		expectedNewMS           *clusterv1.MachineSet
		expectedOldMSs          []*clusterv1.MachineSet
		expectedUpToDateResults map[string]UpToDateResult
		expectedCreateReason    string
	}{
		{
			Name:                    "Get nil if no MachineSets exist",
			deployment:              deployment,
			msList:                  []*clusterv1.MachineSet{},
			expectedNewMS:           nil,
			expectedOldMSs:          nil,
			expectedUpToDateResults: nil,
			expectedCreateReason:    "no MachineSets exist for the MachineDeployment",
		},
		{
			Name:           "Get nil if there are no MachineTemplate that matches the intent of the MachineDeployment",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&oldMS},
			expectedNewMS:  nil,
			expectedOldMSs: []*clusterv1.MachineSet{&oldMS},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					LogMessages:              []string{"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required"},
					ConditionMessages:        []string{"InfrastructureMachine is not up-to-date"},
					EligibleForInPlaceUpdate: true,
				},
			},
			expectedCreateReason: fmt.Sprintf(`couldn't find MachineSet matching MachineDeployment spec template: MachineSet %s: diff: spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required`, oldMS.Name),
		},
		{
			Name:           "Get the MachineSet with the MachineTemplate that matches the intent of the MachineDeployment",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&oldMS, &matchingMS},
			expectedNewMS:  &matchingMS,
			expectedOldMSs: []*clusterv1.MachineSet{&oldMS},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					LogMessages:              []string{"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required"},
					ConditionMessages:        []string{"InfrastructureMachine is not up-to-date"},
					EligibleForInPlaceUpdate: true,
				},
				matchingMS.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:           "Get empty old MachineSets",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&matchingMS},
			expectedNewMS:  &matchingMS,
			expectedOldMSs: []*clusterv1.MachineSet{},
			expectedUpToDateResults: map[string]UpToDateResult{
				matchingMS.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:           "Get the MachineSet with the higher replicas if multiple MachineSets match the desired intent on the MachineDeployment",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&oldMS, &matchingMS, &matchingMSHigherReplicas},
			expectedNewMS:  &matchingMSHigherReplicas,
			expectedOldMSs: []*clusterv1.MachineSet{&oldMS, &matchingMS},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					LogMessages:              []string{"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required"},
					ConditionMessages:        []string{"InfrastructureMachine is not up-to-date"},
					EligibleForInPlaceUpdate: true,
				},
				matchingMS.Name: {
					EligibleForInPlaceUpdate: false,
				},
				matchingMSHigherReplicas.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:           "Get the MachineSet with the MachineTemplate that matches the desired intent on the MachineDeployment, except differs in in-place mutable fields",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&oldMS, &matchingMSDiffersInPlaceMutableFields},
			expectedNewMS:  &matchingMSDiffersInPlaceMutableFields,
			expectedOldMSs: []*clusterv1.MachineSet{&oldMS},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					LogMessages:              []string{"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required"},
					ConditionMessages:        []string{"InfrastructureMachine is not up-to-date"},
					EligibleForInPlaceUpdate: true,
				},
				matchingMSDiffersInPlaceMutableFields.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:           "Get nil if no MachineSet matches the desired intent of the MachineDeployment",
			deployment:     deployment,
			msList:         []*clusterv1.MachineSet{&oldMS},
			expectedNewMS:  nil,
			expectedOldMSs: []*clusterv1.MachineSet{&oldMS},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					LogMessages:              []string{"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required"},
					ConditionMessages:        []string{"InfrastructureMachine is not up-to-date"},
					EligibleForInPlaceUpdate: true,
				},
			},
			expectedCreateReason: fmt.Sprintf(`couldn't find MachineSet matching MachineDeployment spec template: MachineSet %s: diff: spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required`, oldMS.Name),
		},
		{
			Name:               "Get nil if no MachineSet matches the desired intent of the MachineDeployment, reconciliationTime is > rolloutAfter",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&oldMSCreatedThreeBeforeRolloutAfter},
			reconciliationTime: oneAfterRolloutAfter,
			expectedNewMS:      nil,
			expectedOldMSs:     []*clusterv1.MachineSet{&oldMSCreatedThreeBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				oldMS.Name: {
					ConditionMessages: []string{"InfrastructureMachine is not up-to-date"},
					LogMessages: []string{
						// An additional message must be added to old machine sets when reconciliationTime is > rolloutAfter.
						"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required",
						"MachineDeployment spec.rolloutAfter expired",
					},
					// EligibleForInPlaceUpdate decision should change for oldMS when reconciliationTime is > rolloutAfter.
					EligibleForInPlaceUpdate: false,
				},
			},
			expectedCreateReason: fmt.Sprintf(`couldn't find MachineSet matching MachineDeployment spec template: MachineSet %s: diff: spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required, MachineDeployment spec.rolloutAfter expired`, oldMS.Name),
		},
		{
			Name:               "Get the MachineSet if reconciliationTime < rolloutAfter",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter, &msCreatedThreeBeforeRolloutAfter},
			reconciliationTime: oneBeforeRolloutAfter,
			expectedNewMS:      &msCreatedThreeBeforeRolloutAfter,
			expectedOldMSs:     []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				msCreatedTwoBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				msCreatedThreeBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:               "Get nil if reconciliationTime is > rolloutAfter and no MachineSet is created after rolloutAfter",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter, &msCreatedThreeBeforeRolloutAfter, &oldMSCreatedThreeBeforeRolloutAfter},
			reconciliationTime: oneAfterRolloutAfter,
			expectedNewMS:      nil,
			expectedOldMSs:     []*clusterv1.MachineSet{&oldMSCreatedThreeBeforeRolloutAfter, &msCreatedThreeBeforeRolloutAfter, &msCreatedTwoBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				msCreatedTwoBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				msCreatedThreeBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				oldMS.Name: {
					ConditionMessages: []string{"InfrastructureMachine is not up-to-date"},
					LogMessages: []string{
						"spec.infrastructureRef InfrastructureMachineTemplate old-infra-ref, InfrastructureMachineTemplate new-infra-ref required",
						// An additional message must be added to old machine sets when reconciliationTime is > rolloutAfter.
						"MachineDeployment spec.rolloutAfter expired",
					},
					// EligibleForInPlaceUpdate decision should change for oldMS when reconciliationTime is > rolloutAfter.
					EligibleForInPlaceUpdate: false,
				},
			},
			expectedCreateReason: fmt.Sprintf("spec.rollout.after on MachineDeployment set to %s, no MachineSet has been created afterwards", rolloutAfter.Format(time.RFC3339)),
		},
		{
			Name:               "Get MachineSet created after RolloutAfter if reconciliationTime is > rolloutAfter",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&msCreatedAfterRolloutAfter, &msCreatedTwoBeforeRolloutAfter},
			reconciliationTime: twoAfterRolloutAfter,
			expectedNewMS:      &msCreatedAfterRolloutAfter,
			expectedOldMSs:     []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				msCreatedTwoBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				msCreatedAfterRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			// https://github.com/kubernetes-sigs/cluster-api/issues/12260
			Name:               "Get MachineSet created exactly in RolloutAfter if reconciliationTime > rolloutAfter",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&msCreatedExactlyInRolloutAfter, &msCreatedTwoBeforeRolloutAfter},
			reconciliationTime: oneAfterRolloutAfter,
			expectedNewMS:      &msCreatedExactlyInRolloutAfter,
			expectedOldMSs:     []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				msCreatedTwoBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				msCreatedExactlyInRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
		{
			Name:               "Get MachineSet created after RolloutAfter if reconciliationTime is > rolloutAfter (inverse order in ms list)",
			deployment:         *deploymentWithRolloutAfter,
			msList:             []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter, &msCreatedAfterRolloutAfter},
			reconciliationTime: twoAfterRolloutAfter,
			expectedNewMS:      &msCreatedAfterRolloutAfter,
			expectedOldMSs:     []*clusterv1.MachineSet{&msCreatedTwoBeforeRolloutAfter},
			expectedUpToDateResults: map[string]UpToDateResult{
				msCreatedTwoBeforeRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
				msCreatedAfterRolloutAfter.Name: {
					EligibleForInPlaceUpdate: false,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			g := NewWithT(t)

			newMS, oldMSs, upToDateResults, createReason := FindNewAndOldMachineSets(&test.deployment, test.msList, test.reconciliationTime)
			g.Expect(newMS).To(BeComparableTo(test.expectedNewMS))
			g.Expect(oldMSs).To(BeComparableTo(test.expectedOldMSs))
			g.Expect(upToDateResults).To(BeComparableTo(test.expectedUpToDateResults))
			g.Expect(createReason).To(BeComparableTo(test.expectedCreateReason))
		})
	}
}

func TestGetReplicaCountForMachineSets(t *testing.T) {
	ms1 := generateMS(generateDeployment("foo"))
	*(ms1.Spec.Replicas) = 1
	ms1.Status.Replicas = ptr.To[int32](2)
	ms2 := generateMS(generateDeployment("bar"))
	*(ms2.Spec.Replicas) = 5
	ms2.Status.Replicas = ptr.To[int32](3)

	tests := []struct {
		Name           string
		Sets           []*clusterv1.MachineSet
		ExpectedCount  int32
		ExpectedActual int32
		ExpectedTotal  int32
	}{
		{
			Name:           "1:2 Replicas",
			Sets:           []*clusterv1.MachineSet{&ms1},
			ExpectedCount:  1,
			ExpectedActual: 2,
			ExpectedTotal:  2,
		},
		{
			Name:           "6:5 Replicas",
			Sets:           []*clusterv1.MachineSet{&ms1, &ms2},
			ExpectedCount:  6,
			ExpectedActual: 5,
			ExpectedTotal:  7,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(GetReplicaCountForMachineSets(test.Sets)).To(Equal(test.ExpectedCount))
			g.Expect(ptr.Deref(GetActualReplicaCountForMachineSets(test.Sets), 0)).To(Equal(test.ExpectedActual))
			g.Expect(TotalMachineSetsReplicaSum(test.Sets)).To(Equal(test.ExpectedTotal))
		})
	}
}

func TestResolveFenceposts(t *testing.T) {
	tests := []struct {
		maxSurge          string
		maxUnavailable    string
		desired           int32
		expectSurge       int32
		expectUnavailable int32
		expectError       bool
	}{
		{
			maxSurge:          "0%",
			maxUnavailable:    "0%",
			desired:           0,
			expectSurge:       0,
			expectUnavailable: 1,
			expectError:       false,
		},
		{
			maxSurge:          "39%",
			maxUnavailable:    "39%",
			desired:           10,
			expectSurge:       4,
			expectUnavailable: 3,
			expectError:       false,
		},
		{
			maxSurge:          "oops",
			maxUnavailable:    "39%",
			desired:           10,
			expectSurge:       0,
			expectUnavailable: 0,
			expectError:       true,
		},
		{
			maxSurge:          "55%",
			maxUnavailable:    "urg",
			desired:           10,
			expectSurge:       0,
			expectUnavailable: 0,
			expectError:       true,
		},
		{
			maxSurge:          "5",
			maxUnavailable:    "1",
			desired:           7,
			expectSurge:       0,
			expectUnavailable: 0,
			expectError:       true,
		},
	}

	for _, test := range tests {
		t.Run("maxSurge="+test.maxSurge, func(t *testing.T) {
			g := NewWithT(t)

			maxSurge := intstr.FromString(test.maxSurge)
			maxUnavail := intstr.FromString(test.maxUnavailable)
			surge, unavail, err := ResolveFenceposts(&maxSurge, &maxUnavail, test.desired)
			if test.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(surge).To(Equal(test.expectSurge))
			g.Expect(unavail).To(Equal(test.expectUnavailable))
		})
	}
}

func TestNewMSNewReplicas(t *testing.T) {
	tests := []struct {
		Name          string
		strategyType  clusterv1.MachineDeploymentRolloutStrategyType
		depReplicas   int32
		newMSReplicas int32
		maxSurge      int32
		expected      int32
	}{
		{
			"can not scale up - to newMSReplicas",
			clusterv1.RollingUpdateMachineDeploymentStrategyType,
			1, 5, 1, 5,
		},
		{
			"scale up - to depReplicas",
			clusterv1.RollingUpdateMachineDeploymentStrategyType,
			6, 2, 10, 6,
		},
	}
	newDeployment := generateDeployment("nginx")
	newRC := generateMS(newDeployment)
	rs5 := generateMS(newDeployment)
	*(rs5.Spec.Replicas) = 5

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			g := NewWithT(t)

			*(newDeployment.Spec.Replicas) = test.depReplicas
			newDeployment.Spec.Rollout.Strategy.Type = test.strategyType
			newDeployment.Spec.Rollout.Strategy.RollingUpdate = clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
				MaxUnavailable: ptr.To(intstr.FromInt32(1)),
				MaxSurge:       ptr.To(intstr.FromInt32(test.maxSurge)),
			}
			*(newRC.Spec.Replicas) = test.newMSReplicas
			ms, err := NewMSNewReplicas(&newDeployment, []*clusterv1.MachineSet{&rs5}, *newRC.Spec.Replicas)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(ms).To(Equal(test.expected))
		})
	}
}

func TestDeploymentComplete(t *testing.T) {
	deployment := func(desired, current, upToDate, available, maxUnavailable, maxSurge int32) *clusterv1.MachineDeployment {
		return &clusterv1.MachineDeployment{
			Spec: clusterv1.MachineDeploymentSpec{
				Replicas: &desired,
				Rollout: clusterv1.MachineDeploymentRolloutSpec{
					Strategy: clusterv1.MachineDeploymentRolloutStrategy{
						RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
							MaxUnavailable: ptr.To(intstr.FromInt32(maxUnavailable)),
							MaxSurge:       ptr.To(intstr.FromInt32(maxSurge)),
						},
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					},
				},
			},
			Status: clusterv1.MachineDeploymentStatus{
				Replicas:          ptr.To[int32](current),
				UpToDateReplicas:  ptr.To[int32](upToDate),
				AvailableReplicas: ptr.To[int32](available),
			},
		}
	}

	tests := []struct {
		name string

		md *clusterv1.MachineDeployment

		expected bool
	}{
		{
			name: "not complete: min but not all machines become available",

			md:       deployment(5, 5, 5, 4, 1, 0),
			expected: false,
		},
		{
			name: "not complete: min availability is not honored",

			md:       deployment(5, 5, 5, 3, 1, 0),
			expected: false,
		},
		{
			name: "complete",

			md:       deployment(5, 5, 5, 5, 0, 0),
			expected: true,
		},
		{
			name: "not complete: all machines are available but not updated",

			md:       deployment(5, 5, 4, 5, 0, 0),
			expected: false,
		},
		{
			name: "not complete: still running old machines",

			// old machine set: spec.replicas=1, status.replicas=1, status.availableReplicas=1
			// new machine set: spec.replicas=1, status.replicas=1, status.availableReplicas=0
			md:       deployment(1, 2, 1, 1, 0, 1),
			expected: false,
		},
		{
			name: "not complete: one replica deployment never comes up",

			md:       deployment(1, 1, 1, 0, 1, 1),
			expected: false,
		},
	}

	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(DeploymentComplete(test.md, &test.md.Status)).To(Equal(test.expected))
		})
	}
}

func TestMaxUnavailable(t *testing.T) {
	deployment := func(replicas int32, maxUnavailable intstr.IntOrString) clusterv1.MachineDeployment {
		return clusterv1.MachineDeployment{
			Spec: clusterv1.MachineDeploymentSpec{
				Replicas: func(i int32) *int32 { return &i }(replicas),
				Rollout: clusterv1.MachineDeploymentRolloutSpec{
					Strategy: clusterv1.MachineDeploymentRolloutStrategy{
						RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
							MaxSurge:       ptr.To(intstr.FromInt32(1)),
							MaxUnavailable: &maxUnavailable,
						},
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					},
				},
			},
		}
	}
	tests := []struct {
		name       string
		deployment clusterv1.MachineDeployment
		expected   int32
	}{
		{
			name:       "maxUnavailable less than replicas",
			deployment: deployment(10, intstr.FromInt32(5)),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable equal replicas",
			deployment: deployment(10, intstr.FromInt32(10)),
			expected:   int32(10),
		},
		{
			name:       "maxUnavailable greater than replicas",
			deployment: deployment(5, intstr.FromInt32(10)),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable with replicas is 0",
			deployment: deployment(0, intstr.FromInt32(10)),
			expected:   int32(0),
		},
		{
			name:       "maxUnavailable less than replicas with percents",
			deployment: deployment(10, intstr.FromString("50%")),
			expected:   int32(5),
		},
		{
			name:       "maxUnavailable equal replicas with percents",
			deployment: deployment(10, intstr.FromString("100%")),
			expected:   int32(10),
		},
		{
			name:       "maxUnavailable greater than replicas with percents",
			deployment: deployment(5, intstr.FromString("100%")),
			expected:   int32(5),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(MaxUnavailable(test.deployment)).To(Equal(test.expected))
		})
	}
}

func TestMachineSetAnnotationsFromMachineDeployment(t *testing.T) {
	tDeployment := generateDeployment("nginx")
	tDeployment.Annotations = map[string]string{
		// annotations to skip
		corev1.LastAppliedConfigAnnotation:  "foo",
		clusterv1.RevisionAnnotation:        "foo",
		revisionHistoryAnnotation:           "foo",
		clusterv1.DesiredReplicasAnnotation: "foo",
		clusterv1.MaxReplicasAnnotation:     "foo",
		conversion.DataAnnotation:           "foo",

		// annotations to preserve
		"bar": "bar",
	}
	tDeployment.Spec.Replicas = ptr.To[int32](3)
	tDeployment.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
		Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
		RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
			MaxSurge:       ptr.To(intstr.FromInt32(1)),
			MaxUnavailable: ptr.To(intstr.FromInt32(0)),
		},
	}

	t.Run("Drops well-known annotations, keeps other, adds replica annotations", func(t *testing.T) {
		g := NewWithT(t)

		annotations := MachineSetAnnotationsFromMachineDeployment(ctx, &tDeployment)

		g.Expect(annotations).To(Equal(map[string]string{
			// Drops well-known annotations

			// Keeps other
			"bar": "bar",

			// Adds replica annotations
			clusterv1.DesiredReplicasAnnotation: "3",
			clusterv1.MaxReplicasAnnotation:     "4",
		}))
	})
}

func TestIsSaturated(t *testing.T) {
	tDeployment := generateDeployment("nginx")
	tDeployment.Spec.Replicas = ptr.To[int32](3)

	tMS := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				clusterv1.DesiredReplicasAnnotation: "3",
			},
		},
	}

	t.Run("deployment not yet saturated, ms doesn't have all the desired replicas", func(t *testing.T) {
		g := NewWithT(t)
		tMS := tMS.DeepCopy()
		tMS.Spec.Replicas = ptr.To[int32](1)
		g.Expect(IsSaturated(&tDeployment, tMS)).To(BeFalse())
	})
	t.Run("deployment not yet saturated, ms has all replicas but some are not available yet", func(t *testing.T) {
		g := NewWithT(t)
		tMS := tMS.DeepCopy()
		tMS.Spec.Replicas = ptr.To[int32](3)
		tMS.Status.AvailableReplicas = ptr.To[int32](1)
		g.Expect(IsSaturated(&tDeployment, tMS)).To(BeFalse())
	})
	t.Run("deployment saturated, ms has all replicas and all are available", func(t *testing.T) {
		g := NewWithT(t)
		tMS := tMS.DeepCopy()
		tMS.Spec.Replicas = ptr.To[int32](3)
		tMS.Status.AvailableReplicas = ptr.To[int32](3)
		g.Expect(IsSaturated(&tDeployment, tMS)).To(BeTrue())
	})
}

func TestComputeRevisionAnnotations(t *testing.T) {
	tests := []struct {
		name         string
		oldMSs       []*clusterv1.MachineSet
		ms           *clusterv1.MachineSet
		want         map[string]string
		wantRevision string
		wantErr      bool
	}{
		{
			name:   "Calculating annotations for a new newMS - oldMSs do not exist",
			oldMSs: nil,
			ms:     nil,
			want: map[string]string{
				clusterv1.RevisionAnnotation: "1",
			},
			wantRevision: "1",
			wantErr:      false,
		},
		{
			name:   "Calculating annotations for a new newMS - old MSs exist",
			oldMSs: []*clusterv1.MachineSet{machineSetWithRevisionAndHistory("1", "")},
			ms:     nil,
			want: map[string]string{
				clusterv1.RevisionAnnotation: "2",
			},
			wantRevision: "2",
			wantErr:      false,
		},
		{
			name:   "Calculating annotations for a existing newMS - oldMSs do not exist",
			oldMSs: nil,
			ms:     machineSetWithRevisionAndHistory("1", ""),
			want: map[string]string{
				clusterv1.RevisionAnnotation: "1",
			},
			wantRevision: "1",
			wantErr:      false,
		},
		{
			name: "Calculating annotations for a existing newMS - old MSs exist - update required",
			oldMSs: []*clusterv1.MachineSet{
				machineSetWithRevisionAndHistory("1", ""),
				machineSetWithRevisionAndHistory("2", ""),
			},
			ms: machineSetWithRevisionAndHistory("1", ""),
			want: map[string]string{
				clusterv1.RevisionAnnotation: "3",
				revisionHistoryAnnotation:    "1",
			},
			wantRevision: "3",
			wantErr:      false,
		},
		{
			name: "Calculating annotations for a existing newMS - old MSs exist - no update required",
			oldMSs: []*clusterv1.MachineSet{
				machineSetWithRevisionAndHistory("1", ""),
				machineSetWithRevisionAndHistory("2", ""),
			},
			ms: machineSetWithRevisionAndHistory("4", ""),
			want: map[string]string{
				clusterv1.RevisionAnnotation: "4",
			},
			wantRevision: "4",
			wantErr:      false,
		},
		{
			name: "Calculating annotations for a existing newMS with revision history - old MSs exist - update required",
			oldMSs: []*clusterv1.MachineSet{
				machineSetWithRevisionAndHistory("3", ""),
				machineSetWithRevisionAndHistory("4", ""),
			},
			ms: machineSetWithRevisionAndHistory("2", "1"),
			want: map[string]string{
				clusterv1.RevisionAnnotation: "5",
				revisionHistoryAnnotation:    "1,2",
			},
			wantRevision: "5",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, gotRevision, err := ComputeRevisionAnnotations(ctx, tt.ms, tt.oldMSs)
			if tt.wantErr {
				g.Expect(err).ShouldNot(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got).Should(Equal(tt.want))
				g.Expect(gotRevision).Should(Equal(tt.wantRevision))
			}
		})
	}
}

func TestGetRevisionAnnotations(t *testing.T) {
	t.Run("gets revision annotations", func(t *testing.T) {
		g := NewWithT(t)
		ms := machineSetWithRevisionAndHistory("2", "1")

		annotations := GetRevisionAnnotations(ctx, ms)

		g.Expect(annotations).To(HaveLen(2))
		g.Expect(annotations).To(HaveKeyWithValue(clusterv1.RevisionAnnotation, "2"))
		g.Expect(annotations).To(HaveKeyWithValue(revisionHistoryAnnotation, "1"))
	})
}

func machineSetWithRevisionAndHistory(revision string, revisionHistory string) *clusterv1.MachineSet {
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				clusterv1.RevisionAnnotation: revision,
			},
		},
	}
	if revisionHistory != "" {
		ms.Annotations[revisionHistoryAnnotation] = revisionHistory
	}
	return ms
}
