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

package machinedeployment

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/hash"
	"sigs.k8s.io/cluster-api/util/annotations"
)

type rolloutPlanner struct {
	md       *clusterv1.MachineDeployment
	revision string

	originalMS map[string]*clusterv1.MachineSet
	machines   []*clusterv1.Machine

	newMS        *clusterv1.MachineSet
	createReason string

	oldMSs          []*clusterv1.MachineSet
	upToDateResults map[string]mdutil.UpToDateResult

	scaleIntents     map[string]int32
	computeDesiredMS func(ctx context.Context, deployment *clusterv1.MachineDeployment, currentMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error)
}

func newRolloutPlanner() *rolloutPlanner {
	return &rolloutPlanner{
		scaleIntents:     make(map[string]int32),
		computeDesiredMS: computeDesiredMS,
	}
}

// init rollout planner internal state by taking care of:
//   - Identifying newMS and oldMSs
//   - Create the newMS if it not exists
//   - Compute the initial version of desired state for newMS and oldMSs with mandatory labels, in place propagated fields
//     and the annotations derived from the MachineDeployment.
//
// Note: rollout planner might change desired state later on in the planning phase, e.g. add/remove annotations influencing
// how to perform scale up/down operations.
func (p *rolloutPlanner) init(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, machines []*clusterv1.Machine, createNewMSIfNotExist bool, mdTemplateExists bool) error {
	if md == nil {
		return errors.New("machineDeployment is nil, this is unexpected")
	}

	if md.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineDeployment %v is nil, this is unexpected", client.ObjectKeyFromObject(p.md))
	}

	for _, ms := range msList {
		if ms.Spec.Replicas == nil {
			return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(ms))
		}
	}

	// Store md and machines.
	p.md = md
	p.machines = machines

	// Store original MS, for usage later with SSA patches / SSA caching.
	p.originalMS = make(map[string]*clusterv1.MachineSet)
	for _, ms := range msList {
		p.originalMS[ms.Name] = ms.DeepCopy()
	}

	// Try to find a MachineSet which matches the MachineDeployments intent, the newMS; consider all the other MachineSets as oldMs.
	// NOTE: Fields propagated in-place from the MD are not considered by the comparison, they are not relevant for the rollout decision.
	// NOTE: Expiration of MD rolloutAfter is relevant for the rollout decision, and thus it is considered in FindNewAndOldMachineSets.
	currentNewMS, currentOldMSs, upToDateResults, createReason := mdutil.FindNewAndOldMachineSets(md, msList, metav1.Now())
	p.upToDateResults = upToDateResults

	// Compute desired state for the old MS, with mandatory labels, fields in-place propagated from the MachineDeployment etc.
	for _, currentOldMS := range currentOldMSs {
		desiredOldMS, err := p.computeDesiredOldMS(ctx, currentOldMS)
		if err != nil {
			return err
		}
		p.oldMSs = append(p.oldMSs, desiredOldMS)
	}

	// If there is a current NewMS, compute the desired state for it with mandatory labels, fields in-place propagated from the MachineDeployment etc.
	if currentNewMS != nil {
		desiredNewMS, err := p.computeDesiredNewMS(ctx, currentNewMS)
		if err != nil {
			return err
		}
		p.newMS = desiredNewMS
		return nil
	}

	// If there is no current NewMS, create one if required and possible.
	if !createNewMSIfNotExist {
		return nil
	}

	if !mdTemplateExists {
		return errors.New("cannot create a new MachineSet when templates do not exist")
	}

	// Compute a new MachineSet with mandatory labels, fields in-place propagated from the MachineDeployment etc.
	desiredNewMS, err := p.computeDesiredNewMS(ctx, nil)
	if err != nil {
		return err
	}
	p.newMS = desiredNewMS
	p.createReason = createReason
	return nil
}

// computeDesiredNewMS with mandatory labels, in place propagated fields and the annotations derived from the MachineDeployment.
// Additionally, this procedure ensure the annotations tracking revisions numbers on the newMS is upToDate.
// Note: because we are using Server-Side-Apply we always have to calculate the full object.
func (p *rolloutPlanner) computeDesiredNewMS(ctx context.Context, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	desiredNewMS, err := p.computeDesiredMS(ctx, p.md, currentNewMS)
	if err != nil {
		return nil, err
	}

	// For newMS, make sure the revision annotation has the highest revision number across all MS + update the revision history annotation accordingly.
	revisionAnnotations, revision, err := mdutil.ComputeRevisionAnnotations(ctx, currentNewMS, p.oldMSs)
	if err != nil {
		return nil, err
	}
	annotations.AddAnnotations(desiredNewMS, revisionAnnotations)
	p.revision = revision

	// Always allow creation of machines on newMS.
	desiredNewMS.Annotations[clusterv1.DisableMachineCreateAnnotation] = "false"
	return desiredNewMS, nil
}

// computeDesiredOldMS with mandatory labels, in place propagated fields and the annotations derived from the MachineDeployment.
// Additionally, this procedure ensure the annotations tracking revisions numbers are carried over.
// Note: because we are using Server-Side-Apply we always have to calculate the full object.
func (p *rolloutPlanner) computeDesiredOldMS(ctx context.Context, currentOldMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	desiredOldMS, err := p.computeDesiredMS(ctx, p.md, currentOldMS)
	if err != nil {
		return nil, err
	}

	// For oldMS, carry over the revision annotations (those annotations should not be updated for oldMSs).
	revisionAnnotations := mdutil.GetRevisionAnnotations(ctx, currentOldMS)
	annotations.AddAnnotations(desiredOldMS, revisionAnnotations)

	// Disable creation of machines on oldMS when rollout strategy is on delete.
	if desiredOldMS.Annotations == nil {
		desiredOldMS.Annotations = map[string]string{}
	}
	if p.md.Spec.Rollout.Strategy.Type == clusterv1.OnDeleteMachineDeploymentStrategyType {
		desiredOldMS.Annotations[clusterv1.DisableMachineCreateAnnotation] = "true"
	} else {
		desiredOldMS.Annotations[clusterv1.DisableMachineCreateAnnotation] = "false"
	}
	return desiredOldMS, nil
}

// computeDesiredMS computes the desired MachineSet, which could be either a newly created newMS, or the new desired version of an existing newMS/OldMS.
// Note: because we are using Server-Side-Apply we always have to calculate the full object.
func computeDesiredMS(ctx context.Context, deployment *clusterv1.MachineDeployment, currentMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	var name string
	var uid types.UID
	var finalizers []string
	var uniqueIdentifierLabelValue string
	var machineTemplateSpec clusterv1.MachineSpec
	var status clusterv1.MachineSetStatus
	var replicas int32
	var creationTimestamp metav1.Time

	if currentMS == nil {
		// For a new MachineSet: compute a new uniqueIdentifier, a new MachineSet name, finalizers, replicas and machine template spec (take the one from MachineDeployment)
		// Note: Replicas count might be updated by the rollout planner later in the same reconcile or in following reconcile.

		// Note: In previous Cluster API versions (< v1.4.0), the label value was the hash of the full machine
		// template. Since the introduction of in-place mutation we are ignoring all in-place mutable fields,
		// and using it as a info to be used for building a unique label selector. Instead, the rollout decision
		// is not using the hash anymore.
		templateHash, err := hash.Compute(mdutil.MachineTemplateDeepCopyRolloutFields(&deployment.Spec.Template))
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute desired MachineSet: failed to compute machine template hash")
		}
		// Append a random string at the end of template hash. This is required to distinguish MachineSets that
		// could be created with the same spec as a result of rolloutAfter.
		var randomSuffix string
		name, randomSuffix = computeNewMachineSetName(deployment.Name + "-")
		uniqueIdentifierLabelValue = fmt.Sprintf("%d-%s", templateHash, randomSuffix)
		replicas = 0
		machineTemplateSpec = *deployment.Spec.Template.Spec.DeepCopy()
		creationTimestamp = metav1.NewTime(time.Now())
	} else {
		// For updating an existing MachineSet use name, uid, finalizers, replicas, uniqueIdentifier and machine template spec from existingMS.
		// Note: We use the uid, to ensure that the Server-Side-Apply only updates the existingMS.
		// Note: Replicas count might be updated by the rollout planner later in the same reconcile or in following reconcile.
		var uniqueIdentifierLabelExists bool
		uniqueIdentifierLabelValue, uniqueIdentifierLabelExists = currentMS.Labels[clusterv1.MachineDeploymentUniqueLabel]
		if !uniqueIdentifierLabelExists {
			return nil, errors.Errorf("failed to compute desired MachineSet: failed to get unique identifier from %q annotation",
				clusterv1.MachineDeploymentUniqueLabel)
		}

		name = currentMS.Name
		uid = currentMS.UID
		// Preserve all existing finalizers (including foregroundDeletion finalizer).
		finalizers = currentMS.Finalizers
		replicas = *currentMS.Spec.Replicas
		machineTemplateSpec = *currentMS.Spec.Template.Spec.DeepCopy()
		status = currentMS.Status
		creationTimestamp = currentMS.CreationTimestamp
	}

	// Construct the basic MachineSet.
	desiredMS := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: deployment.Namespace,
			// NOTE: Carry over creationTimestamp from current MS, because it is required by the sorting functions
			// used in the planning phase, e.g. MachineSetsByCreationTimestamp.
			// NOTE: For newMS, this value is set to now, but it will be overridden when actual creation happens
			// NOTE: CreationTimestamp will be dropped from the SSA intent by the SSA helper.
			CreationTimestamp: creationTimestamp,
			// Note: By setting the ownerRef on creation we signal to the MachineSet controller that this is not a stand-alone MachineSet.
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(deployment, machineDeploymentKind)},
			UID:             uid,
			Finalizers:      finalizers,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas:    &replicas,
			ClusterName: deployment.Spec.ClusterName,
			Template: clusterv1.MachineTemplateSpec{
				Spec: machineTemplateSpec,
			},
		},
		// NOTE: Carry over status from current MS, because it is required by mdutil functions
		// used in the planning phase, e.g. GetAvailableReplicaCountForMachineSets.
		// NOTE: Status will be dropped from the SSA intent by the SSA helper.
		Status: status,
	}

	// Set the in-place mutable fields.
	// When we create a new MachineSet we will just create the MachineSet with those fields.
	// When we update an existing MachineSet will we update the fields on the existing MachineSet (in-place mutate).

	// Set labels and .spec.template.labels.
	desiredMS.Labels = mdutil.CloneAndAddLabel(deployment.Spec.Template.Labels,
		clusterv1.MachineDeploymentUniqueLabel, uniqueIdentifierLabelValue)

	// Always set the MachineDeploymentNameLabel.
	// Note: If a client tries to create a MachineDeployment without a selector, the MachineDeployment webhook
	// will add this label automatically. But we want this label to always be present even if the MachineDeployment
	// has a selector which doesn't include it. Therefore, we have to set it here explicitly.
	desiredMS.Labels[clusterv1.MachineDeploymentNameLabel] = deployment.Name
	desiredMS.Spec.Template.Labels = mdutil.CloneAndAddLabel(deployment.Spec.Template.Labels,
		clusterv1.MachineDeploymentUniqueLabel, uniqueIdentifierLabelValue)

	// Set selector.
	desiredMS.Spec.Selector = *mdutil.CloneSelectorAndAddLabel(&deployment.Spec.Selector, clusterv1.MachineDeploymentUniqueLabel, uniqueIdentifierLabelValue)

	// Set annotations and .spec.template.annotations.
	// Note: Additional annotations might be added by the rollout planner later in the same reconcile.
	// Note: Intentionally, we are not setting the following labels:
	// - clusterv1.RevisionAnnotation + the deprecated revisionHistoryAnnotation
	//   - for newMS, we should always keep those annotations upToDate
	//   - for oldMS, we should carry over those annotations from previous reconcile
	// - clusterv1.DisableMachineCreateAnnotation
	//	 - it should be set to true only on oldMS and if strategy is on delete, otherwise set to false.
	desiredMS.Annotations = mdutil.MachineSetAnnotationsFromMachineDeployment(ctx, deployment)
	desiredMS.Spec.Template.Annotations = cloneStringMap(deployment.Spec.Template.Annotations)

	// Set all other in-place mutable fields.
	desiredMS.Spec.Deletion.Order = deployment.Spec.Deletion.Order
	desiredMS.Spec.MachineNaming = deployment.Spec.MachineNaming
	desiredMS.Spec.Template.Spec.MinReadySeconds = deployment.Spec.Template.Spec.MinReadySeconds
	desiredMS.Spec.Template.Spec.ReadinessGates = deployment.Spec.Template.Spec.ReadinessGates
	desiredMS.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds
	desiredMS.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds
	desiredMS.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = deployment.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds

	return desiredMS, nil
}
