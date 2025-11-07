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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apirand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// sync is responsible for reconciling deployments on scaling events or when they
// are paused.
func (r *Reconciler) sync(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, machines collections.Machines, templateExists bool) error {
	// Use the rollout planner to take benefit of the common logic for:
	// - identifying newMS and OldMS when necessary
	// - computing desired state for newMS and OldMS, including managing rollout related annotations and
	//   in-place propagation of labels, annotations and other fields.
	planner := newRolloutPlanner(r.Client, r.RuntimeClient)
	if err := planner.init(ctx, md, msList, machines.UnsortedList(), false, templateExists); err != nil {
		return err
	}

	// Applying above changes to MachineSets, so it will be possible to use legacy code for scale.
	if err := r.createOrUpdateMachineSetsAndSyncMachineDeploymentRevision(ctx, planner); err != nil {
		return err
	}

	// Call the legacy scale logic.
	// Note: the legacy scale logic do not relies yet on the rollout planner, and it still lead to many
	// patch calls vs grouping all the MachineSet changes in a single SSA call based on a carefully crafted desired state.
	// Note: using the legacy scale logic on newMS and oldMSs computed by the rollout planner is not an issue because
	// the legacy scale logic relies on info that are part of the desired state computed by rollout planner (or of
	// info carried over from original MS). More specifically:
	// - ms.metadata.CreationTimestamp, carried over
	// - ms.metadata.Annotations, computed (only DesiredReplicasAnnotation, MaxReplicasAnnotation are relevant)
	// - ms.spec.Replicas, computed
	// - ms.status.Replicas, carried over
	// - ms.status.AvailableReplicas, carried over
	newMS := planner.newMS
	oldMSs := planner.oldMSs
	allMSs := append(oldMSs, newMS)

	// TODO(in-place): consider if to move the scale logic to the rollout planner as well, so we can improve test coverage
	//  like we did for RolloutUpdate and OnDelete strategy.
	if err := r.scale(ctx, md, newMS, oldMSs); err != nil {
		// If we get an error while trying to scale, the deployment will be requeued
		// so we can abort this resync
		return err
	}

	return r.syncDeploymentStatus(allMSs, newMS, md)
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
	// Set replica counters on MD status.
	setReplicas(md, allMSs)
	calculateV1Beta1Status(allMSs, newMS, md)

	// minReplicasNeeded will be equal to md.Spec.Replicas when the strategy is not RollingUpdateMachineDeploymentStrategyType.
	minReplicasNeeded := *(md.Spec.Replicas) - mdutil.MaxUnavailable(*md)

	availableReplicas := int32(0)
	if md.Status.Deprecated != nil && md.Status.Deprecated.V1Beta1 != nil {
		availableReplicas = md.Status.Deprecated.V1Beta1.AvailableReplicas
	}
	if availableReplicas >= minReplicasNeeded {
		// NOTE: The structure of calculateV1Beta1Status() does not allow us to update the machinedeployment directly, we can only update the status obj it returns. Ideally, we should change calculateV1Beta1Status() --> updateStatus() to be consistent with the rest of the code base, until then, we update conditions here.
		v1beta1conditions.MarkTrue(md, clusterv1.MachineDeploymentAvailableV1Beta1Condition)
	} else {
		v1beta1conditions.MarkFalse(md, clusterv1.MachineDeploymentAvailableV1Beta1Condition, clusterv1.WaitingForAvailableMachinesV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Minimum availability requires %d replicas, current %d available", minReplicasNeeded, ptr.Deref(md.Status.AvailableReplicas, 0))
	}

	if newMS != nil {
		// Report a summary of current status of the MachineSet object owned by this MachineDeployment.
		v1beta1conditions.SetMirror(md, clusterv1.MachineSetReadyV1Beta1Condition,
			newMS,
			v1beta1conditions.WithFallbackValue(false, clusterv1.WaitingForMachineSetFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, ""),
		)
	} else {
		v1beta1conditions.MarkFalse(md, clusterv1.MachineSetReadyV1Beta1Condition, clusterv1.WaitingForMachineSetFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, "MachineSet not found")
	}

	return nil
}

// calculateV1Beta1Status calculates the latest status for the provided deployment by looking into the provided MachineSets.
func calculateV1Beta1Status(allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) {
	availableReplicas := mdutil.GetV1Beta1AvailableReplicaCountForMachineSets(allMSs)
	totalReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)
	unavailableReplicas := totalReplicas - availableReplicas

	// If unavailableReplicas is negative, then that means the Deployment has more available replicas running than
	// desired, e.g. whenever it scales down. In such a case we should simply default unavailableReplicas to zero.
	if unavailableReplicas < 0 {
		unavailableReplicas = 0
	}

	if deployment.Status.Deprecated == nil {
		deployment.Status.Deprecated = &clusterv1.MachineDeploymentDeprecatedStatus{}
	}
	if deployment.Status.Deprecated.V1Beta1 == nil {
		deployment.Status.Deprecated.V1Beta1 = &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{}
	}

	deployment.Status.Deprecated.V1Beta1.UpdatedReplicas = ptr.Deref(mdutil.GetActualReplicaCountForMachineSets([]*clusterv1.MachineSet{newMS}), 0)
	deployment.Status.Deprecated.V1Beta1.ReadyReplicas = mdutil.GetV1Beta1ReadyReplicaCountForMachineSets(allMSs)
	deployment.Status.Deprecated.V1Beta1.AvailableReplicas = availableReplicas
	deployment.Status.Deprecated.V1Beta1.UnavailableReplicas = unavailableReplicas
}

func (r *Reconciler) scaleMachineSet(ctx context.Context, ms *clusterv1.MachineSet, newScale int32, deployment *clusterv1.MachineDeployment) error {
	if ms.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(ms))
	}

	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineDeployment %v is nil, this is unexpected", client.ObjectKeyFromObject(deployment))
	}

	// No need to scale, return.
	if *(ms.Spec.Replicas) == newScale {
		return nil
	}

	// If we're here, a scaling operation is required.
	patchHelper, err := patch.NewHelper(ms, r.Client)
	if err != nil {
		return err
	}

	// Save original replicas to log in event.
	originalReplicas := *(ms.Spec.Replicas)

	// Mutate replicas.
	ms.Spec.Replicas = &newScale

	if err := patchHelper.Patch(ctx, ms); err != nil {
		r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedScale", "Failed to scale MachineSet %v: %v",
			client.ObjectKeyFromObject(ms), err)
		return err
	}

	r.recorder.Eventf(deployment, corev1.EventTypeNormal, "SuccessfulScale", "Scaled MachineSet %v: %d -> %d",
		client.ObjectKeyFromObject(ms), originalReplicas, *ms.Spec.Replicas)

	return nil
}

// cleanupDeployment is responsible for cleaning up a MachineDeployment.
func (r *Reconciler) cleanupDeployment(ctx context.Context, oldMSs []*clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) error {
	log := ctrl.LoggerFrom(ctx)

	// Avoid deleting machine set with deletion timestamp set
	aliveFilter := func(ms *clusterv1.MachineSet) bool {
		return ms != nil && ms.DeletionTimestamp.IsZero()
	}

	cleanableMSes := mdutil.FilterMachineSets(oldMSs, aliveFilter)

	cleanableMSCount := int32(len(cleanableMSes))
	if cleanableMSCount == 0 {
		return nil
	}

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(cleanableMSes))
	log.V(4).Info("Looking to cleanup old machine sets for deployment")

	for i := range cleanableMSCount {
		ms := cleanableMSes[i]
		if ms.Spec.Replicas == nil {
			return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", ms.Name)
		}

		// Avoid delete machine set with non-zero replica counts
		if ptr.Deref(ms.Status.Replicas, 0) != 0 || *(ms.Spec.Replicas) != 0 || ms.Generation > ms.Status.ObservedGeneration || !ms.DeletionTimestamp.IsZero() {
			continue
		}

		log.V(4).Info("Trying to cleanup machine set for deployment", "MachineSet", klog.KObj(ms))
		if err := r.Client.Delete(ctx, ms); err != nil && !apierrors.IsNotFound(err) {
			// Return error instead of aggregating and continuing DELETEs on the theory
			// that we may be overloading the api server.
			r.recorder.Eventf(deployment, corev1.EventTypeWarning, "FailedDelete", "Failed to delete MachineSet %q: %v", ms.Name, err)
			return err
		}
		// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
		log.Info("Deleting MachineSet (cleanup of old MachineSet)", "MachineSet", klog.KObj(ms))
		r.recorder.Eventf(deployment, corev1.EventTypeNormal, "SuccessfulDelete", "Deleted MachineSet %q", ms.Name)
	}

	return nil
}
