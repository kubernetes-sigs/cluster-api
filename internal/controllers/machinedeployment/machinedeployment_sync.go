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
	"sort"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/internal/util/hash"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

// sync is responsible for reconciling deployments on scaling events or when they
// are paused.
func (r *Reconciler) sync(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, templateExists bool) error {
	newMS, oldMSs, err := r.getAllMachineSetsAndSyncRevision(ctx, md, msList, false, templateExists)
	if err != nil {
		return err
	}

	if err := r.scale(ctx, md, newMS, oldMSs); err != nil {
		// If we get an error while trying to scale, the deployment will be requeued
		// so we can abort this resync
		return err
	}

	//
	// // TODO: Clean up the deployment when it's paused and no rollback is in flight.
	//
	allMSs := append(oldMSs, newMS)
	return r.syncDeploymentStatus(allMSs, newMS, md)
}

// getAllMachineSetsAndSyncRevision returns all the machine sets for the provided deployment (new and all old), with new MS's and deployment's revision updated.
//
// msList should come from getMachineSetsForDeployment(d).
// machineMap should come from getMachineMapForDeployment(d, msList).
//
//  1. Get all old MSes this deployment targets, and calculate the max revision number among them (maxOldV).
//  2. Get new MS this deployment targets (whose machine template matches deployment's), and update new MS's revision number to (maxOldV + 1),
//     only if its revision number is smaller than (maxOldV + 1). If this step failed, we'll update it in the next deployment sync loop.
//  3. Copy new MS's revision number to deployment (update deployment's revision). If this step failed, we'll update it in the next deployment sync loop.
//
// Note that currently the deployment controller is using caches to avoid querying the server for reads.
// This may lead to stale reads of machine sets, thus incorrect deployment status.
func (r *Reconciler) getAllMachineSetsAndSyncRevision(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, createIfNotExisted, templateExists bool) (*clusterv1.MachineSet, []*clusterv1.MachineSet, error) {
	reconciliationTime := metav1.Now()
	allOldMSs, err := mdutil.FindOldMachineSets(md, msList, &reconciliationTime)
	if err != nil {
		return nil, nil, err
	}

	// Get new machine set with the updated revision number
	newMS, err := r.getNewMachineSet(ctx, md, msList, allOldMSs, createIfNotExisted, templateExists, &reconciliationTime)
	if err != nil {
		return nil, nil, err
	}

	return newMS, allOldMSs, nil
}

// Returns a MachineSet that matches the intent of the given MachineDeployment.
// If there does not exist such a MachineSet and createIfNotExisted is true, create a new MachineSet.
// If there is already such a MachineSet, update it to propagate in-place mutable fields from the MachineDeployment.
func (r *Reconciler) getNewMachineSet(ctx context.Context, md *clusterv1.MachineDeployment, msList, oldMSs []*clusterv1.MachineSet, createIfNotExists, templateExists bool, reconciliationTime *metav1.Time) (*clusterv1.MachineSet, error) {
	// Try to find a MachineSet which matches the MachineDeployments intent, while ignore diffs between
	// the in-place mutable fields.
	// If we find a matching MachineSet we just update it to propagate any changes to the in-place mutable
	// fields and thus we do not trigger an unnecessary rollout (i.e. create a new MachineSet).
	// If we don't find a matching MachineSet, we need a rollout and thus create a new MachineSet.
	// Note: The in-place mutable fields can be just updated inline, because they do not affect the actual machines
	// themselves (i.e. the infrastructure and the software running on the Machines not the Machine object).
	matchingMS, createReason, err := mdutil.FindNewMachineSet(md, msList, reconciliationTime)
	if err != nil {
		return nil, err
	}

	// If there is a MachineSet that matches the intent of the MachineDeployment, update the MachineSet
	// to propagate all in-place mutable fields from MachineDeployment to the MachineSet.
	if matchingMS != nil {
		updatedMS, err := r.updateMachineSet(ctx, md, matchingMS, oldMSs)
		if err != nil {
			return nil, err
		}

		// Ensure MachineDeployment has the latest MachineSet revision in its revision annotation.
		mdutil.SetDeploymentRevision(md, updatedMS.Annotations[clusterv1.RevisionAnnotation])
		return updatedMS, nil
	}

	if !createIfNotExists {
		return nil, nil
	}

	if !templateExists {
		return nil, errors.New("cannot create a new MachineSet when templates do not exist")
	}

	// Create a new MachineSet and wait until the new MachineSet exists in the cache.
	newMS, err := r.createMachineSetAndWait(ctx, md, oldMSs, createReason)
	if err != nil {
		return nil, err
	}

	mdutil.SetDeploymentRevision(md, newMS.Annotations[clusterv1.RevisionAnnotation])

	return newMS, nil
}

// updateMachineSet updates an existing MachineSet to propagate in-place mutable fields from the MachineDeployment.
func (r *Reconciler) updateMachineSet(ctx context.Context, deployment *clusterv1.MachineDeployment, ms *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	log := ctrl.LoggerFrom(ctx)

	// Compute the desired MachineSet.
	updatedMS, err := r.computeDesiredMachineSet(ctx, deployment, ms, oldMSs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update MachineSet %q", klog.KObj(ms))
	}

	// Update the MachineSet to propagate in-place mutable fields from the MachineDeployment.
	err = ssa.Patch(ctx, r.Client, machineDeploymentManagerName, updatedMS, ssa.WithCachingProxy{Cache: r.ssaCache, Original: ms})
	if err != nil {
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedUpdate", "Failed to update MachineSet %s: %v", klog.KObj(updatedMS), err)
		return nil, errors.Wrapf(err, "failed to update MachineSet %s", klog.KObj(updatedMS))
	}

	log.V(4).Info("Updated MachineSet", "MachineSet", klog.KObj(updatedMS))
	return updatedMS, nil
}

// createMachineSetAndWait creates a new MachineSet with the desired intent of the MachineDeployment.
// It waits for the cache to be updated with the newly created MachineSet.
func (r *Reconciler) createMachineSetAndWait(ctx context.Context, deployment *clusterv1.MachineDeployment, oldMSs []*clusterv1.MachineSet, createReason string) (*clusterv1.MachineSet, error) {
	log := ctrl.LoggerFrom(ctx)

	// Compute the desired MachineSet.
	newMS, err := r.computeDesiredMachineSet(ctx, deployment, nil, oldMSs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new MachineSet")
	}

	log = log.WithValues("MachineSet", klog.KObj(newMS))
	ctx = ctrl.LoggerInto(ctx, log)

	log.Info(fmt.Sprintf("Creating new MachineSet, %s", createReason))

	// Create the MachineSet.
	if err := ssa.Patch(ctx, r.Client, machineDeploymentManagerName, newMS); err != nil {
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedCreate", "Failed to create MachineSet %s: %v", klog.KObj(newMS), err)
		return nil, errors.Wrapf(err, "failed to create new MachineSet %s", klog.KObj(newMS))
	}
	log.V(4).Info("Created new MachineSet")
	r.recorder.Eventf(deployment, corev1.EventTypeNormal, "SuccessfulCreate", "Created MachineSet %s", klog.KObj(newMS))

	// Keep trying to get the MachineSet. This will force the cache to update and prevent any future reconciliation of
	// the MachineDeployment to reconcile with an outdated list of MachineSets which could lead to unwanted creation of
	// a duplicate MachineSet.
	var pollErrors []error
	if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		ms := &clusterv1.MachineSet{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(newMS), ms); err != nil {
			// Do not return error here. Continue to poll even if we hit an error
			// so that we avoid existing because of transient errors like network flakes.
			// Capture all the errors and return the aggregate error if the poll fails eventually.
			pollErrors = append(pollErrors, err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, errors.Wrapf(kerrors.NewAggregate(pollErrors), "failed to get the MachineSet %s after creation", klog.KObj(newMS))
	}
	return newMS, nil
}

// computeDesiredMachineSet computes the desired MachineSet.
// This MachineSet will be used during reconciliation to:
// * create a MachineSet
// * update an existing MachineSet
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the MachineSet depending on if it
// is a create or update. Example: for a new MachineSet we have to calculate a new name,
// while for an existing MachineSet we have to use the name of the existing MachineSet.
func (r *Reconciler) computeDesiredMachineSet(ctx context.Context, deployment *clusterv1.MachineDeployment, existingMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	var name string
	var uid types.UID
	var finalizers []string
	var uniqueIdentifierLabelValue string
	var machineTemplateSpec clusterv1.MachineSpec
	var replicas int32
	var err error

	// For a new MachineSet:
	// * compute a new uniqueIdentifier, a new MachineSet name, finalizers, replicas and
	//   machine template spec (take the one from MachineDeployment)
	if existingMS == nil {
		// Note: In previous Cluster API versions (< v1.4.0), the label value was the hash of the full machine
		// template. With the introduction of in-place mutation the machine template of the MachineSet can change.
		// Because of that it is impossible that the label's value to always be the hash of the full machine template.
		// (Because the hash changes when the machine template changes).
		// As a result, we use the hash of the machine template while ignoring all in-place mutable fields, i.e. the
		// machine template with only fields that could trigger a rollout for the machine-template-hash, making it
		// independent of the changes to any in-place mutable fields.
		templateHash, err := hash.Compute(mdutil.MachineTemplateDeepCopyRolloutFields(&deployment.Spec.Template))
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute desired MachineSet: failed to compute machine template hash")
		}
		// Append a random string at the end of template hash. This is required to distinguish MachineSets that
		// could be created with the same spec as a result of rolloutAfter. If not, computeDesiredMachineSet
		// will end up updating the existing MachineSet instead of creating a new one.
		var randomSuffix string
		name, randomSuffix = computeNewMachineSetName(deployment.Name + "-")
		uniqueIdentifierLabelValue = fmt.Sprintf("%d-%s", templateHash, randomSuffix)

		replicas, err = mdutil.NewMSNewReplicas(deployment, oldMSs, 0)
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute desired MachineSet")
		}

		machineTemplateSpec = *deployment.Spec.Template.Spec.DeepCopy()
	} else {
		// For updating an existing MachineSet:
		// * get the uniqueIdentifier from labels of the existingMS
		// * use name, uid, finalizers, replicas and machine template spec from existingMS.
		// Note: We use the uid, to ensure that the Server-Side-Apply only updates existingMS.
		// Note: We carry over those fields because we don't want to mutate them for an existingMS.
		var uniqueIdentifierLabelExists bool
		uniqueIdentifierLabelValue, uniqueIdentifierLabelExists = existingMS.Labels[clusterv1.MachineDeploymentUniqueLabel]
		if !uniqueIdentifierLabelExists {
			return nil, errors.Errorf("failed to compute desired MachineSet: failed to get unique identifier from %q annotation",
				clusterv1.MachineDeploymentUniqueLabel)
		}

		name = existingMS.Name
		uid = existingMS.UID

		// Preserve all existing finalizers (including foregroundDeletion finalizer).
		finalizers = existingMS.Finalizers

		replicas = *existingMS.Spec.Replicas

		machineTemplateSpec = *existingMS.Spec.Template.Spec.DeepCopy()
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
	if desiredMS.Annotations, err = mdutil.ComputeMachineSetAnnotations(ctx, deployment, oldMSs, existingMS); err != nil {
		return nil, errors.Wrap(err, "failed to compute desired MachineSet: failed to compute annotations")
	}
	desiredMS.Spec.Template.Annotations = cloneStringMap(deployment.Spec.Template.Annotations)

	// Set all other in-place mutable fields.
	desiredMS.Spec.MinReadySeconds = ptr.Deref(deployment.Spec.MinReadySeconds, 0)
	if deployment.Spec.Strategy != nil && deployment.Spec.Strategy.RollingUpdate != nil {
		desiredMS.Spec.DeletePolicy = ptr.Deref(deployment.Spec.Strategy.RollingUpdate.DeletePolicy, "")
	} else {
		desiredMS.Spec.DeletePolicy = ""
	}
	desiredMS.Spec.Template.Spec.ReadinessGates = deployment.Spec.Template.Spec.ReadinessGates
	desiredMS.Spec.Template.Spec.NodeDrainTimeout = deployment.Spec.Template.Spec.NodeDrainTimeout
	desiredMS.Spec.Template.Spec.NodeDeletionTimeout = deployment.Spec.Template.Spec.NodeDeletionTimeout
	desiredMS.Spec.Template.Spec.NodeVolumeDetachTimeout = deployment.Spec.Template.Spec.NodeVolumeDetachTimeout

	return desiredMS, nil
}

// cloneStringMap clones a string map.
func cloneStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

const (
	maxNameLength          = 63
	randomLength           = 5
	maxGeneratedNameLength = maxNameLength - randomLength
)

// computeNewMachineSetName generates a new name for the MachineSet just like
// the upstream SimpleNameGenerator.
// Note: We had to extract the logic as we want to use the MachineSet name suffix as
// unique identifier for the MachineSet.
func computeNewMachineSetName(base string) (string, string) {
	if len(base) > maxGeneratedNameLength {
		base = base[:maxGeneratedNameLength]
	}
	r := apirand.String(randomLength)
	return fmt.Sprintf("%s%s", base, r), r
}

// scale scales proportionally in order to mitigate risk. Otherwise, scaling up can increase the size
// of the new machine set and scaling down can decrease the sizes of the old ones, both of which would
// have the effect of hastening the rollout progress, which could produce a higher proportion of unavailable
// replicas in the event of a problem with the rolled out template. Should run only on scaling events or
// when a deployment is paused and not during the normal rollout process.
func (r *Reconciler) scale(ctx context.Context, deployment *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx)

	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for deployment %v is nil, this is unexpected", deployment.Name)
	}

	// If there is only one active machine set then we should scale that up to the full count of the
	// deployment. If there is no active machine set, then we should scale up the newest machine set.
	if activeOrLatest := mdutil.FindOneActiveOrLatest(newMS, oldMSs); activeOrLatest != nil {
		if activeOrLatest.Spec.Replicas == nil {
			return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", activeOrLatest.Name)
		}

		if *(activeOrLatest.Spec.Replicas) == *(deployment.Spec.Replicas) {
			return nil
		}

		err := r.scaleMachineSet(ctx, activeOrLatest, *(deployment.Spec.Replicas), deployment)
		return err
	}

	// If the new machine set is saturated, old machine sets should be fully scaled down.
	// This case handles machine set adoption during a saturated new machine set.
	if mdutil.IsSaturated(deployment, newMS) {
		for _, old := range mdutil.FilterActiveMachineSets(oldMSs) {
			if err := r.scaleMachineSet(ctx, old, 0, deployment); err != nil {
				return err
			}
		}
		return nil
	}

	// There are old machine sets with machines and the new machine set is not saturated.
	// We need to proportionally scale all machine sets (new and old) in case of a
	// rolling deployment.
	if mdutil.IsRollingUpdate(deployment) {
		allMSs := mdutil.FilterActiveMachineSets(append(oldMSs, newMS))
		totalMSReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)

		allowedSize := int32(0)
		if *(deployment.Spec.Replicas) > 0 {
			allowedSize = *(deployment.Spec.Replicas) + mdutil.MaxSurge(*deployment)
		}

		// Number of additional replicas that can be either added or removed from the total
		// replicas count. These replicas should be distributed proportionally to the active
		// machine sets.
		deploymentReplicasToAdd := allowedSize - totalMSReplicas

		// The additional replicas should be distributed proportionally amongst the active
		// machine sets from the larger to the smaller in size machine set. Scaling direction
		// drives what happens in case we are trying to scale machine sets of the same size.
		// In such a case when scaling up, we should scale up newer machine sets first, and
		// when scaling down, we should scale down older machine sets first.
		switch {
		case deploymentReplicasToAdd > 0:
			sort.Sort(mdutil.MachineSetsBySizeNewer(allMSs))
		case deploymentReplicasToAdd < 0:
			sort.Sort(mdutil.MachineSetsBySizeOlder(allMSs))
		}

		// Iterate over all active machine sets and estimate proportions for each of them.
		// The absolute value of deploymentReplicasAdded should never exceed the absolute
		// value of deploymentReplicasToAdd.
		deploymentReplicasAdded := int32(0)
		nameToSize := make(map[string]int32)
		for i := range allMSs {
			ms := allMSs[i]
			if ms.Spec.Replicas == nil {
				log.Info("Spec.Replicas for machine set is nil, this is unexpected.", "MachineSet", ms.Name)
				continue
			}

			// Estimate proportions if we have replicas to add, otherwise simply populate
			// nameToSize with the current sizes for each machine set.
			if deploymentReplicasToAdd != 0 {
				proportion := mdutil.GetProportion(ms, *deployment, deploymentReplicasToAdd, deploymentReplicasAdded, log)
				nameToSize[ms.Name] = *(ms.Spec.Replicas) + proportion
				deploymentReplicasAdded += proportion
			} else {
				nameToSize[ms.Name] = *(ms.Spec.Replicas)
			}
		}

		// Update all machine sets
		for i := range allMSs {
			ms := allMSs[i]

			// Add/remove any leftovers to the largest machine set.
			if i == 0 && deploymentReplicasToAdd != 0 {
				leftover := deploymentReplicasToAdd - deploymentReplicasAdded
				nameToSize[ms.Name] += leftover
				if nameToSize[ms.Name] < 0 {
					nameToSize[ms.Name] = 0
				}
			}

			if err := r.scaleMachineSet(ctx, ms, nameToSize[ms.Name], deployment); err != nil {
				// Return as soon as we fail, the deployment is requeued
				return err
			}
		}
	}

	return nil
}

// syncDeploymentStatus checks if the status is up-to-date and sync it if necessary.
func (r *Reconciler) syncDeploymentStatus(allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet, md *clusterv1.MachineDeployment) error {
	md.Status = calculateStatus(allMSs, newMS, md)

	// minReplicasNeeded will be equal to md.Spec.Replicas when the strategy is not RollingUpdateMachineDeploymentStrategyType.
	minReplicasNeeded := *(md.Spec.Replicas) - mdutil.MaxUnavailable(*md)

	if md.Status.AvailableReplicas >= minReplicasNeeded {
		// NOTE: The structure of calculateStatus() does not allow us to update the machinedeployment directly, we can only update the status obj it returns. Ideally, we should change calculateStatus() --> updateStatus() to be consistent with the rest of the code base, until then, we update conditions here.
		conditions.MarkTrue(md, clusterv1.MachineDeploymentAvailableCondition)
	} else {
		conditions.MarkFalse(md, clusterv1.MachineDeploymentAvailableCondition, clusterv1.WaitingForAvailableMachinesReason, clusterv1.ConditionSeverityWarning, "Minimum availability requires %d replicas, current %d available", minReplicasNeeded, md.Status.AvailableReplicas)
	}

	if newMS != nil {
		// Report a summary of current status of the MachineSet object owned by this MachineDeployment.
		conditions.SetMirror(md, clusterv1.MachineSetReadyCondition,
			newMS,
			conditions.WithFallbackValue(false, clusterv1.WaitingForMachineSetFallbackReason, clusterv1.ConditionSeverityInfo, ""),
		)
	} else {
		conditions.MarkFalse(md, clusterv1.MachineSetReadyCondition, clusterv1.WaitingForMachineSetFallbackReason, clusterv1.ConditionSeverityInfo, "MachineSet not found")
	}

	// Set v1beta replica counters on MD status.
	setReplicas(md, allMSs)

	return nil
}

// calculateStatus calculates the latest status for the provided deployment by looking into the provided MachineSets.
func calculateStatus(allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) clusterv1.MachineDeploymentStatus {
	availableReplicas := mdutil.GetAvailableReplicaCountForMachineSets(allMSs)
	totalReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)
	unavailableReplicas := totalReplicas - availableReplicas

	// If unavailableReplicas is negative, then that means the Deployment has more available replicas running than
	// desired, e.g. whenever it scales down. In such a case we should simply default unavailableReplicas to zero.
	if unavailableReplicas < 0 {
		unavailableReplicas = 0
	}

	// Calculate the label selector. We check the error in the MD reconcile function, ignore here.
	selector, _ := metav1.LabelSelectorAsSelector(&deployment.Spec.Selector)

	status := clusterv1.MachineDeploymentStatus{
		// TODO: Ensure that if we start retrying status updates, we won't pick up a new Generation value.
		ObservedGeneration:  deployment.Generation,
		Selector:            selector.String(),
		Replicas:            mdutil.GetActualReplicaCountForMachineSets(allMSs),
		UpdatedReplicas:     mdutil.GetActualReplicaCountForMachineSets([]*clusterv1.MachineSet{newMS}),
		ReadyReplicas:       mdutil.GetReadyReplicaCountForMachineSets(allMSs),
		AvailableReplicas:   availableReplicas,
		UnavailableReplicas: unavailableReplicas,
		Conditions:          deployment.Status.Conditions,

		// preserve v1beta2 status
		V1Beta2: deployment.Status.V1Beta2,
	}

	if *deployment.Spec.Replicas == status.ReadyReplicas {
		status.Phase = string(clusterv1.MachineDeploymentPhaseRunning)
	}
	if *deployment.Spec.Replicas > status.ReadyReplicas {
		status.Phase = string(clusterv1.MachineDeploymentPhaseScalingUp)
	}
	// This is the same as unavailableReplicas, but we have to recalculate because unavailableReplicas
	// would have been reset to zero above if it was negative
	if totalReplicas-availableReplicas < 0 {
		status.Phase = string(clusterv1.MachineDeploymentPhaseScalingDown)
	}
	for _, ms := range allMSs {
		if ms != nil {
			if ms.Status.FailureReason != nil || ms.Status.FailureMessage != nil {
				status.Phase = string(clusterv1.MachineDeploymentPhaseFailed)
				break
			}
		}
	}
	return status
}

func (r *Reconciler) scaleMachineSet(ctx context.Context, ms *clusterv1.MachineSet, newScale int32, deployment *clusterv1.MachineDeployment) error {
	if ms.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(ms))
	}

	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineDeployment %v is nil, this is unexpected", client.ObjectKeyFromObject(deployment))
	}

	annotationsNeedUpdate := mdutil.ReplicasAnnotationsNeedUpdate(
		ms,
		*(deployment.Spec.Replicas),
		*(deployment.Spec.Replicas)+mdutil.MaxSurge(*deployment),
	)

	// No need to scale nor setting annotations, return.
	if *(ms.Spec.Replicas) == newScale && !annotationsNeedUpdate {
		return nil
	}

	// If we're here, a scaling operation is required.
	patchHelper, err := patch.NewHelper(ms, r.Client)
	if err != nil {
		return err
	}

	// Save original replicas to log in event.
	originalReplicas := *(ms.Spec.Replicas)

	// Mutate replicas and the related annotation.
	ms.Spec.Replicas = &newScale
	mdutil.SetReplicasAnnotations(ms, *(deployment.Spec.Replicas), *(deployment.Spec.Replicas)+mdutil.MaxSurge(*deployment))

	if err := patchHelper.Patch(ctx, ms); err != nil {
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedScale", "Failed to scale MachineSet %v: %v",
			client.ObjectKeyFromObject(ms), err)
		return err
	}

	r.recorder.Eventf(deployment, corev1.EventTypeNormal, "SuccessfulScale", "Scaled MachineSet %v: %d -> %d",
		client.ObjectKeyFromObject(ms), originalReplicas, *ms.Spec.Replicas)

	return nil
}

// cleanupDeployment is responsible for cleaning up a deployment i.e. retains all but the latest N old machine sets
// where N=d.Spec.RevisionHistoryLimit. Old machine sets are older versions of the machinetemplate of a deployment kept
// around by default 1) for historical reasons and 2) for the ability to rollback a deployment.
func (r *Reconciler) cleanupDeployment(ctx context.Context, oldMSs []*clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) error {
	log := ctrl.LoggerFrom(ctx)

	if deployment.Spec.RevisionHistoryLimit == nil {
		return nil
	}

	// Avoid deleting machine set with deletion timestamp set
	aliveFilter := func(ms *clusterv1.MachineSet) bool {
		return ms != nil && ms.ObjectMeta.DeletionTimestamp.IsZero()
	}

	cleanableMSes := mdutil.FilterMachineSets(oldMSs, aliveFilter)

	diff := int32(len(cleanableMSes)) - *deployment.Spec.RevisionHistoryLimit
	if diff <= 0 {
		return nil
	}

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(cleanableMSes))
	log.V(4).Info("Looking to cleanup old machine sets for deployment")

	for i := range diff {
		ms := cleanableMSes[i]
		if ms.Spec.Replicas == nil {
			return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", ms.Name)
		}

		// Avoid delete machine set with non-zero replica counts
		if ms.Status.Replicas != 0 || *(ms.Spec.Replicas) != 0 || ms.Generation > ms.Status.ObservedGeneration || !ms.DeletionTimestamp.IsZero() {
			continue
		}

		log.V(4).Info("Trying to cleanup machine set for deployment", "MachineSet", klog.KObj(ms))
		if err := r.Client.Delete(ctx, ms); err != nil && !apierrors.IsNotFound(err) {
			// Return error instead of aggregating and continuing DELETEs on the theory
			// that we may be overloading the api server.
			r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedDelete", "Failed to delete MachineSet %q: %v", ms.Name, err)
			return err
		}
		r.recorder.Eventf(deployment, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted MachineSet %q", ms.Name)
	}

	return nil
}
