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
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/hash"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/cache"
)

type rolloutPlanner struct {
	Client                   client.Client
	RuntimeClient            runtimeclient.Client
	canUpdateMachineSetCache cache.Cache[CanUpdateMachineSetCacheEntry]

	md       *clusterv1.MachineDeployment
	revision string

	originalMSs              map[string]*clusterv1.MachineSet
	machines                 []*clusterv1.Machine
	acknowledgedMachineNames []string
	updatingMachineNames     []string

	newMS        *clusterv1.MachineSet
	createReason string

	oldMSs          []*clusterv1.MachineSet
	upToDateResults map[string]mdutil.UpToDateResult

	scaleIntents map[string]int32
	notes        map[string][]string

	overrideComputeDesiredMS              func(ctx context.Context, deployment *clusterv1.MachineDeployment, currentMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error)
	overrideCanUpdateMachineSetInPlace    func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet) (bool, error)
	overrideCanExtensionsUpdateMachineSet func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (bool, []string, error)
}

func newRolloutPlanner(c client.Client, runtimeClient runtimeclient.Client, canUpdateMachineSetCache cache.Cache[CanUpdateMachineSetCacheEntry]) *rolloutPlanner {
	return &rolloutPlanner{
		Client:                   c,
		RuntimeClient:            runtimeClient,
		canUpdateMachineSetCache: canUpdateMachineSetCache,
		scaleIntents:             make(map[string]int32),
		notes:                    make(map[string][]string),
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
	p.originalMSs = make(map[string]*clusterv1.MachineSet)
	for _, ms := range msList {
		p.originalMSs[ms.Name] = ms.DeepCopy()
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
		oldRevision := currentNewMS.Annotations[clusterv1.RevisionAnnotation]
		desiredNewMS, err := p.computeDesiredNewMS(ctx, currentNewMS)
		if err != nil {
			return err
		}
		p.newMS = desiredNewMS
		// Note: when an oldMS becomes again the current one, computeDesiredNewMS changes its revision number;
		// the new revision number is also surfaced in p.revision, and thus we are using this information
		// to determine when to add this note.
		if oldRevision != p.revision {
			p.addNote(p.newMS, "this is now the current MachineSet")
		}
		return nil
	}

	// If there is no current NewMS, create one if required and possible.
	if !createNewMSIfNotExist {
		return nil
	}

	if !mdTemplateExists {
		return errors.New("cannot create a MachineSet when templates do not exist")
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
	computeFunc := computeDesiredMS
	if p.overrideComputeDesiredMS != nil {
		computeFunc = p.overrideComputeDesiredMS
	}
	desiredNewMS, err := computeFunc(ctx, p.md, currentNewMS)
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
	computeFunc := computeDesiredMS
	if p.overrideComputeDesiredMS != nil {
		computeFunc = p.overrideComputeDesiredMS
	}
	desiredOldMS, err := computeFunc(ctx, p.md, currentOldMS)
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
	desiredMS.Spec.Template.Spec.Taints = deployment.Spec.Template.Spec.Taints

	return desiredMS, nil
}

func (p *rolloutPlanner) addNote(ms *clusterv1.MachineSet, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	for _, note := range p.notes[ms.Name] {
		if note == msg {
			return
		}
	}
	p.notes[ms.Name] = append(p.notes[ms.Name], msg)
}

type machineSetDiff struct {
	Type             string
	OriginalMS       *clusterv1.MachineSet
	OriginalReplicas int32
	DesiredReplicas  int32
	Summary          string
	OtherChanges     string
	Reason           string
}

func (p *rolloutPlanner) getMachineSetDiff(ms *clusterv1.MachineSet) machineSetDiff {
	diff := machineSetDiff{}

	// Get the originalMS, if any.
	originalMS, hasOriginalMS := p.originalMSs[ms.Name]
	if hasOriginalMS {
		diff.OriginalMS = originalMS
	}

	// Determine if this is the current MS or an old one.
	// Note: using "current" for logging instead of "new" to avoid confusion between new/current and new/last created.
	diff.Type = "old"
	if ms.Name == p.newMS.Name {
		diff.Type = "current"
	}

	// Retrieve original spec.replicas
	// Note: Used to determine if this is a scale up or a scale down operation.
	if hasOriginalMS {
		diff.OriginalReplicas = ptr.Deref(originalMS.Spec.Replicas, 0)
	}

	// Compute the desired spec.replicas
	// Note: Used to determine if this is a scale up or a scale down operation.
	diff.DesiredReplicas = ptr.Deref(ms.Spec.Replicas, 0)

	// Compute a message that summarize changes to Machine set spec replicas.
	// Note. Also other replicas counters not changed by the rollout planner are included, because they are used in the computation.
	if !hasOriginalMS || diff.OriginalReplicas == diff.DesiredReplicas {
		diff.Summary = fmt.Sprintf("%d/%d replicas", ptr.Deref(ms.Status.Replicas, 0), diff.DesiredReplicas)
	} else {
		diff.Summary = fmt.Sprintf("from %d/%d to %d/%d replicas", ptr.Deref(ms.Status.Replicas, 0), diff.OriginalReplicas, ptr.Deref(ms.Status.Replicas, 0), diff.DesiredReplicas)
	}
	diff.Summary += fmt.Sprintf(", %d available, %d upToDate", ptr.Deref(ms.Status.AvailableReplicas, 0), ptr.Deref(ms.Status.UpToDateReplicas, 0))

	// Compute a message with the detailed changes between current and original MS (only fields/annotation relevant for the rollout decision are included).
	changes := []string{}
	if hasOriginalMS && diff.OriginalReplicas != diff.DesiredReplicas {
		changes = append(changes, fmt.Sprintf("replicas %d", diff.DesiredReplicas))
	}
	if hasOriginalMS {
		if originalMS.Annotations[clusterv1.RevisionAnnotation] != ms.Annotations[clusterv1.RevisionAnnotation] {
			if value, ok := ms.Annotations[clusterv1.RevisionAnnotation]; ok {
				changes = append(changes, fmt.Sprintf("%s: %s", clusterv1.RevisionAnnotation, value))
			}
		}

		if originalMS.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation] != ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation] {
			if value, ok := ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation]; ok {
				changes = append(changes, fmt.Sprintf("%s: %s", clusterv1.MachineSetMoveMachinesToMachineSetAnnotation, value))
			} else {
				changes = append(changes, fmt.Sprintf("%s removed", clusterv1.MachineSetMoveMachinesToMachineSetAnnotation))
			}
		}

		if originalMS.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation] != ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation] {
			if value, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok {
				changes = append(changes, fmt.Sprintf("%s: %s", clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation, value))
			} else {
				changes = append(changes, fmt.Sprintf("%s removed", clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation))
			}
		}

		if originalMS.Annotations[clusterv1.AcknowledgedMoveAnnotation] != ms.Annotations[clusterv1.AcknowledgedMoveAnnotation] {
			if value, ok := ms.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok {
				changes = append(changes, fmt.Sprintf("%s: %s", clusterv1.AcknowledgedMoveAnnotation, value))
			} else {
				changes = append(changes, fmt.Sprintf("%s removed", clusterv1.AcknowledgedMoveAnnotation))
			}
		}

		if originalMS.Annotations[clusterv1.DisableMachineCreateAnnotation] != ms.Annotations[clusterv1.DisableMachineCreateAnnotation] {
			if value, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok {
				changes = append(changes, fmt.Sprintf("%s: %s", clusterv1.DisableMachineCreateAnnotation, value))
			} else {
				changes = append(changes, fmt.Sprintf("%s removed", clusterv1.DisableMachineCreateAnnotation))
			}
		}

		diff.OtherChanges = strings.Join(changes, ",")
	}

	// Collect notes explaining the reason why something changed
	diff.Reason = strings.Join(p.notes[ms.Name], ", ")

	return diff
}
