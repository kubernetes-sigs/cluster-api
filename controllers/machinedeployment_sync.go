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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/internal/mdutil"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// sync is responsible for reconciling deployments on scaling events or when they
// are paused.
func (r *MachineDeploymentReconciler) sync(ctx context.Context, d *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) error {
	newMS, oldMSs, err := r.getAllMachineSetsAndSyncRevision(ctx, d, msList, false)
	if err != nil {
		return err
	}

	if err := r.scale(ctx, d, newMS, oldMSs); err != nil {
		// If we get an error while trying to scale, the deployment will be requeued
		// so we can abort this resync
		return err
	}

	//
	// // TODO: Clean up the deployment when it's paused and no rollback is in flight.
	//
	allMSs := append(oldMSs, newMS)
	return r.syncDeploymentStatus(allMSs, newMS, d)
}

// getAllMachineSetsAndSyncRevision returns all the machine sets for the provided deployment (new and all old), with new MS's and deployment's revision updated.
//
// msList should come from getMachineSetsForDeployment(d).
// machineMap should come from getMachineMapForDeployment(d, msList).
//
// 1. Get all old MSes this deployment targets, and calculate the max revision number among them (maxOldV).
// 2. Get new MS this deployment targets (whose machine template matches deployment's), and update new MS's revision number to (maxOldV + 1),
//    only if its revision number is smaller than (maxOldV + 1). If this step failed, we'll update it in the next deployment sync loop.
// 3. Copy new MS's revision number to deployment (update deployment's revision). If this step failed, we'll update it in the next deployment sync loop.
//
// Note that currently the deployment controller is using caches to avoid querying the server for reads.
// This may lead to stale reads of machine sets, thus incorrect deployment status.
func (r *MachineDeploymentReconciler) getAllMachineSetsAndSyncRevision(ctx context.Context, d *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, createIfNotExisted bool) (*clusterv1.MachineSet, []*clusterv1.MachineSet, error) {
	_, allOldMSs := mdutil.FindOldMachineSets(d, msList)

	// Get new machine set with the updated revision number
	newMS, err := r.getNewMachineSet(ctx, d, msList, allOldMSs, createIfNotExisted)
	if err != nil {
		return nil, nil, err
	}

	return newMS, allOldMSs, nil
}

// Returns a machine set that matches the intent of the given deployment. Returns nil if the new machine set doesn't exist yet.
// 1. Get existing new MS (the MS that the given deployment targets, whose machine template is the same as deployment's).
// 2. If there's existing new MS, update its revision number if it's smaller than (maxOldRevision + 1), where maxOldRevision is the max revision number among all old MSes.
// 3. If there's no existing new MS and createIfNotExisted is true, create one with appropriate revision number (maxOldRevision + 1) and replicas.
// Note that the machine-template-hash will be added to adopted MSes and machines.
func (r *MachineDeploymentReconciler) getNewMachineSet(ctx context.Context, d *clusterv1.MachineDeployment, msList, oldMSs []*clusterv1.MachineSet, createIfNotExisted bool) (*clusterv1.MachineSet, error) {
	log := ctrl.LoggerFrom(ctx)

	existingNewMS := mdutil.FindNewMachineSet(d, msList)

	// Calculate the max revision number among all old MSes
	maxOldRevision := mdutil.MaxRevision(oldMSs, log)

	// Calculate revision number for this new machine set
	newRevision := strconv.FormatInt(maxOldRevision+1, 10)

	// Latest machine set exists. We need to sync its annotations (includes copying all but
	// annotationsToSkip from the parent deployment, and update revision, desiredReplicas,
	// and maxReplicas) and also update the revision annotation in the deployment with the
	// latest revision.
	if existingNewMS != nil {
		msCopy := existingNewMS.DeepCopy()
		patchHelper, err := patch.NewHelper(msCopy, r.Client)
		if err != nil {
			return nil, err
		}

		// Set existing new machine set's annotation
		annotationsUpdated := mdutil.SetNewMachineSetAnnotations(d, msCopy, newRevision, true, log)

		minReadySecondsNeedsUpdate := msCopy.Spec.MinReadySeconds != *d.Spec.MinReadySeconds
		deletePolicyNeedsUpdate := d.Spec.Strategy.RollingUpdate.DeletePolicy != nil && msCopy.Spec.DeletePolicy != *d.Spec.Strategy.RollingUpdate.DeletePolicy
		if annotationsUpdated || minReadySecondsNeedsUpdate || deletePolicyNeedsUpdate {
			msCopy.Spec.MinReadySeconds = *d.Spec.MinReadySeconds

			if deletePolicyNeedsUpdate {
				msCopy.Spec.DeletePolicy = *d.Spec.Strategy.RollingUpdate.DeletePolicy
			}

			return nil, patchHelper.Patch(ctx, msCopy)
		}

		// Apply revision annotation from existingNewMS if it is missing from the deployment.
		err = r.updateMachineDeployment(ctx, d, func(innerDeployment *clusterv1.MachineDeployment) {
			mdutil.SetDeploymentRevision(d, msCopy.Annotations[clusterv1.RevisionAnnotation])
		})
		return msCopy, err
	}

	if !createIfNotExisted {
		return nil, nil
	}

	// new MachineSet does not exist, create one.
	newMSTemplate := *d.Spec.Template.DeepCopy()
	hash, err := mdutil.ComputeSpewHash(&newMSTemplate)
	if err != nil {
		return nil, err
	}
	machineTemplateSpecHash := fmt.Sprintf("%d", hash)
	newMSTemplate.Labels = mdutil.CloneAndAddLabel(d.Spec.Template.Labels,
		clusterv1.MachineDeploymentUniqueLabel, machineTemplateSpecHash)

	// Add machineTemplateHash label to selector.
	newMSSelector := mdutil.CloneSelectorAndAddLabel(&d.Spec.Selector,
		clusterv1.MachineDeploymentUniqueLabel, machineTemplateSpecHash)

	minReadySeconds := int32(0)
	if d.Spec.MinReadySeconds != nil {
		minReadySeconds = *d.Spec.MinReadySeconds
	}

	// Create new MachineSet
	newMS := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			// Make the name deterministic, to ensure idempotence
			Name:            d.Name + "-" + apirand.SafeEncodeString(machineTemplateSpecHash),
			Namespace:       d.Namespace,
			Labels:          newMSTemplate.Labels,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(d, machineDeploymentKind)},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName:     d.Spec.ClusterName,
			Replicas:        new(int32),
			MinReadySeconds: minReadySeconds,
			Selector:        *newMSSelector,
			Template:        newMSTemplate,
		},
	}

	if feature.Gates.Enabled(feature.ClusterTopology) {
		// If the MachineDeployment is owned by a Cluster Topology,
		// add the finalizer to allow the topology controller to
		// clean up resources when the MachineSet is deleted.
		// MachineSets are deleted during rollout (e.g. template rotation) and
		// after MachineDeployment deletion.
		if labels.IsTopologyOwned(d) {
			controllerutil.AddFinalizer(&newMS, clusterv1.MachineSetTopologyFinalizer)
		}
	}

	if d.Spec.Strategy.RollingUpdate.DeletePolicy != nil {
		newMS.Spec.DeletePolicy = *d.Spec.Strategy.RollingUpdate.DeletePolicy
	}

	// Add foregroundDeletion finalizer to MachineSet if the MachineDeployment has it
	if sets.NewString(d.Finalizers...).Has(metav1.FinalizerDeleteDependents) {
		controllerutil.AddFinalizer(&newMS, metav1.FinalizerDeleteDependents)
	}

	allMSs := append(oldMSs, &newMS)
	newReplicasCount, err := mdutil.NewMSNewReplicas(d, allMSs, &newMS)
	if err != nil {
		return nil, err
	}

	*(newMS.Spec.Replicas) = newReplicasCount

	// Set new machine set's annotation
	mdutil.SetNewMachineSetAnnotations(d, &newMS, newRevision, false, log)
	// Create the new MachineSet. If it already exists, then we need to check for possible
	// hash collisions. If there is any other error, we need to report it in the status of
	// the Deployment.
	alreadyExists := false
	err = r.Client.Create(ctx, &newMS)
	createdMS := &newMS
	switch {
	// We may end up hitting this due to a slow cache or a fast resync of the Deployment.
	case apierrors.IsAlreadyExists(err):
		alreadyExists = true

		ms := &clusterv1.MachineSet{}
		msErr := r.Client.Get(ctx, client.ObjectKey{Namespace: newMS.Namespace, Name: newMS.Name}, ms)
		if msErr != nil {
			return nil, msErr
		}

		// If the Deployment owns the MachineSet and the MachineSet's MachineTemplateSpec is semantically
		// deep equal to the MachineTemplateSpec of the Deployment, it's the Deployment's new MachineSet.
		// Otherwise, this is a hash collision and we need to increment the collisionCount field in
		// the status of the Deployment and requeue to try the creation in the next sync.
		controllerRef := metav1.GetControllerOf(ms)
		if controllerRef != nil && controllerRef.UID == d.UID && mdutil.EqualMachineTemplate(&d.Spec.Template, &ms.Spec.Template) {
			createdMS = ms
			break
		}

		return nil, err
	case err != nil:
		log.Error(err, "Failed to create new machine set", "machineset", newMS.Name)
		r.recorder.Eventf(d, corev1.EventTypeWarning, "FailedCreate", "Failed to create MachineSet %q: %v", newMS.Name, err)
		return nil, err
	}

	if !alreadyExists {
		log.V(4).Info("Created new machine set", "machineset", createdMS.Name)
		r.recorder.Eventf(d, corev1.EventTypeNormal, "SuccessfulCreate", "Created MachineSet %q", newMS.Name)
	}

	err = r.updateMachineDeployment(ctx, d, func(innerDeployment *clusterv1.MachineDeployment) {
		mdutil.SetDeploymentRevision(d, newRevision)
	})

	return createdMS, err
}

// scale scales proportionally in order to mitigate risk. Otherwise, scaling up can increase the size
// of the new machine set and scaling down can decrease the sizes of the old ones, both of which would
// have the effect of hastening the rollout progress, which could produce a higher proportion of unavailable
// replicas in the event of a problem with the rolled out template. Should run only on scaling events or
// when a deployment is paused and not during the normal rollout process.
func (r *MachineDeploymentReconciler) scale(ctx context.Context, deployment *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) error {
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
				log.Info("Spec.Replicas for machine set is nil, this is unexpected.", "machineset", ms.Name)
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
func (r *MachineDeploymentReconciler) syncDeploymentStatus(allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet, d *clusterv1.MachineDeployment) error {
	d.Status = calculateStatus(allMSs, newMS, d)

	// minReplicasNeeded will be equal to d.Spec.Replicas when the strategy is not RollingUpdateMachineDeploymentStrategyType.
	minReplicasNeeded := *(d.Spec.Replicas) - mdutil.MaxUnavailable(*d)

	if d.Status.AvailableReplicas >= minReplicasNeeded {
		// NOTE: The structure of calculateStatus() does not allow us to update the machinedeployment directly, we can only update the status obj it returns. Ideally, we should change calculateStatus() --> updateStatus() to be consistent with the rest of the code base, until then, we update conditions here.
		conditions.MarkTrue(d, clusterv1.MachineDeploymentAvailableCondition)
	} else {
		conditions.MarkFalse(d, clusterv1.MachineDeploymentAvailableCondition, clusterv1.WaitingForAvailableMachinesReason, clusterv1.ConditionSeverityWarning, "Minimum availability requires %d replicas, current %d available", minReplicasNeeded, d.Status.AvailableReplicas)
	}
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

func (r *MachineDeploymentReconciler) scaleMachineSet(ctx context.Context, ms *clusterv1.MachineSet, newScale int32, deployment *clusterv1.MachineDeployment) error {
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
func (r *MachineDeploymentReconciler) cleanupDeployment(ctx context.Context, oldMSs []*clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) error {
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

	for i := int32(0); i < diff; i++ {
		ms := cleanableMSes[i]
		if ms.Spec.Replicas == nil {
			return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", ms.Name)
		}

		// Avoid delete machine set with non-zero replica counts
		if ms.Status.Replicas != 0 || *(ms.Spec.Replicas) != 0 || ms.Generation > ms.Status.ObservedGeneration || !ms.DeletionTimestamp.IsZero() {
			continue
		}

		log.V(4).Info("Trying to cleanup machine set for deployment", "machineset", ms.Name)
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

func (r *MachineDeploymentReconciler) updateMachineDeployment(ctx context.Context, d *clusterv1.MachineDeployment, modify func(*clusterv1.MachineDeployment)) error {
	return updateMachineDeployment(ctx, r.Client, d, modify)
}

// We have this as standalone variant to be able to use it from the tests.
func updateMachineDeployment(ctx context.Context, c client.Client, d *clusterv1.MachineDeployment, modify func(*clusterv1.MachineDeployment)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := c.Get(ctx, util.ObjectKey(d), d); err != nil {
			return err
		}
		patchHelper, err := patch.NewHelper(d, c)
		if err != nil {
			return err
		}
		clusterv1.PopulateDefaultsMachineDeployment(d)
		modify(d)
		return patchHelper.Patch(ctx, d)
	})
}
