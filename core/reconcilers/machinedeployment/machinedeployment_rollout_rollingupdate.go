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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/core/reconcilers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
	"sigs.k8s.io/cluster-api/util"
)

// rolloutRollingUpdate reconcile machine sets controlled by a MachineDeployment that is using the RolloutUpdate strategy.
// Note: The returned result requests a requeue when the rollout is paused waiting for an answer from the CanUpdateMachineSet extension,
// or blocked because the extension does not allow any operation that can roll out changes.
func (r *Reconciler) rolloutRollingUpdate(ctx context.Context, s *scope, templateExists bool) (ctrl.Result, error) {
	md := s.machineDeployment

	planner := newRolloutPlanner(r.Client, r.RuntimeClient, r.canUpdateMachineSetCache)
	// Note: The results of the CanUpdateMachineSet hook are surfaced in the RollingOut condition, also if the planner returns an error.
	defer func() {
		s.canUpdateMachineSetResults = planner.canUpdateMachineSetResults
	}()
	if err := planner.init(ctx, md, s.machineSets, s.machines.UnsortedList(), true, templateExists); err != nil {
		return ctrl.Result{}, err
	}

	if err := planner.planRollingUpdate(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createOrUpdateMachineSetsAndSyncMachineDeploymentRevision(ctx, planner); err != nil {
		return ctrl.Result{}, err
	}

	newMS := planner.newMS
	oldMSs := planner.oldMSs
	allMSs := append(oldMSs, newMS)

	if err := r.syncDeploymentStatus(allMSs, newMS, md); err != nil {
		return ctrl.Result{}, err
	}

	if mdutil.DeploymentComplete(md, &md.Status) {
		if err := r.cleanupDeployment(ctx, oldMSs, md); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: planner.requeueAfter()}, nil
}

// planRollingUpdate determine how to proceed with the rollout when using the RollingUpdate strategy if the system is not yet at the desired state.
func (p *rolloutPlanner) planRollingUpdate(ctx context.Context) error {
	// Adjust the replica count for the newMS after a move operation has been completed.
	p.reconcileReplicasPendingAcknowledgeMove(ctx)

	// Scale up, if we can.
	if err := p.reconcileNewMachineSet(ctx); err != nil {
		return err
	}

	// Scale down, if we can.
	if err := p.reconcileOldMachineSetsRollingUpdate(ctx); err != nil {
		return err
	}

	// Ensures CAPI rolls out changes by performing in-place updates whenever possible.
	if err := p.reconcileInPlaceUpdateIntent(ctx); err != nil {
		return err
	}

	// This func tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
	// Note: This func must be called after computing scale up/down intent for all the MachineSets.
	// Note: This func only addresses deadlocks due to unavailable replicas not getting deleted on oldMSs, which can happen
	// because reconcileOldMachineSetsRollingUpdate called above always assumes the worst case when deleting replicas e.g.
	//  - MD with spec.replicas 3, MaxSurge 1, MaxUnavailable 0
	//  - OldMS with 3 replicas, 2 available replica (and thus 1 unavailable replica)
	//  - NewMS with 1 replica, 1 available replica
	//  - In theory it is possible to scale down oldMS from 3->2 replicas by deleting the unavailable replica.
	//  - However, reconcileOldMachineSetsRollingUpdate cannot assume that the MachineSet controller is going to delete
	//    the unavailable replica when scaling down from 3->2, because it might happen that one of the available replicas
	//    is deleted instead.
	//  - As a consequence reconcileOldMachineSetsRollingUpdate, which assumes the worst case when deleting replicas, did not scaled down oldMS.
	// This situation, rollout not proceeding due to unavailable replicas, is considered a deadlock to be addressed by reconcileDeadlockBreaker.
	// Note: Unblocking deadlocks when unavailable replicas exist only on oldMSs, is required also because replicas on oldMSs are not remediated by MHC.
	p.reconcileDeadlockBreaker(ctx)
	return nil
}

// reconcileReplicasPendingAcknowledgeMove adjust the replica count for the newMS after a move operation has been completed.
// Note: This operation must be performed before computing scale up/down intent for all the MachineSets (so this operation can take into account also moved machines in the current reconcile).
func (p *rolloutPlanner) reconcileReplicasPendingAcknowledgeMove(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)

	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return
	}

	// Acknowledge replicas after a move operation.
	// NOTE:
	// - The code below also acknowledge replicas moved to on oldMS, to handle the case a machine has been already moved
	//   to the previously newMS, and the previously newMs became an oldMS while the machine is still pending acknowledge move.
	// - It is required to acknowledge replicas moved to on oldMS because the machine has to reach the target state agreed
	//   at the moment move was started before being able to be moved again (or deleted).
	// - Acknowledge move on oldMS does not increase the (desired) replica number for the oldMS.
	// - PendingAcknowledgeMoveAnnotation from machine (managed by the MS controller) and AcknowledgedMoveAnnotation (managed by the rollout planner)
	//   are used in combination to ensure moved replicas are counted only once by the rollout planner.
	var allMSs []*clusterv1.MachineSet
	if p.newMS != nil {
		allMSs = append(allMSs, p.newMS)
	}
	allMSs = append(allMSs, p.oldMSs...)
	for _, ms := range allMSs {
		oldAcknowledgeMoveReplicas := sets.Set[string]{}
		if originalMS, ok := p.originalMSs[ms.Name]; ok {
			if machineNames, ok := originalMS.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok && machineNames != "" {
				oldAcknowledgeMoveReplicas.Insert(strings.Split(machineNames, ",")...)
			}
		}
		newAcknowledgeMoveReplicas := sets.Set[string]{}
		for _, m := range p.machines {
			if !util.IsControlledBy(m, ms, clusterv1.GroupVersion.WithKind("MachineSet").GroupKind()) {
				continue
			}
			if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; !ok {
				continue
			}
			if !oldAcknowledgeMoveReplicas.Has(m.Name) {
				if p.acknowledgedMachineNames == nil {
					p.acknowledgedMachineNames = make(map[string][]string)
				}
				p.acknowledgedMachineNames[ms.Name] = append(p.acknowledgedMachineNames[ms.Name], m.Name)
			}
			newAcknowledgeMoveReplicas.Insert(m.Name)
		}

		if len(p.acknowledgedMachineNames[ms.Name]) > 0 {
			if ms.Name == p.newMS.Name {
				// Note: After this change the replica count will include all the newly acknowledged replicas.
				// Please note that, within the same reconcile, the rollout planner might revisit replicas for the newMS
				// e.g. to account for the Machine deployment being scaled up or down.
				scaleUpCount := int32(len(p.acknowledgedMachineNames[ms.Name]))
				replicaCount := ptr.Deref(ms.Spec.Replicas, 0) + scaleUpCount
				ms.Spec.Replicas = new(replicaCount)
				log.V(5).Info(fmt.Sprintf("Acknowledge replicas %s moved from an old MachineSet. Scale up MachineSet %s to %d (+%d)", sortAndJoin(newAcknowledgeMoveReplicas.UnsortedList()), ms.Name, replicaCount, scaleUpCount), "MachineSet", klog.KObj(ms))
			} else {
				log.V(5).Info(fmt.Sprintf("Acknowledge replicas %s moved from an old MachineSet.", sortAndJoin(newAcknowledgeMoveReplicas.UnsortedList())), "MachineSet", klog.KObj(ms))
			}
			p.addNotef(ms, "acknowledge Machines %s moved from an old MachineSet", sortAndJoin(newAcknowledgeMoveReplicas.UnsortedList()))
		}

		// Track the list or replicas for which acknowledgeMove is not yet completed;
		// The MachineSetController will use this info to clean up the PendingAcknowledgeMoveAnnotation on machines.
		// NOTE: cleanup of the AcknowledgedMoveAnnotation will happen automatically as soon as the rollout planner (this func) stops
		// to set it, because this annotation is not part of the output of computeDesiredMS.
		if ms.Annotations == nil {
			ms.Annotations = map[string]string{}
		}
		if newAcknowledgeMoveReplicas.Len() > 0 {
			ms.Annotations[clusterv1.AcknowledgedMoveAnnotation] = sortAndJoin(newAcknowledgeMoveReplicas.UnsortedList())
		}
	}
}

// reconcileNewMachineSet reconciles the replica number for the new MS.
// Note: In case of scale down this function does not consider the possible impact on availability.
// This is considered acceptable because historically it never led to any problem, but we might revisit this in the future
// because some limitations of this approach are becoming more evident, e.g.
//
//	when users scale down the MD, the operation might temporarily breach min availability (maxUnavailable)
//
// There are code paths specifically added to prevent this issue becoming more relevant when doing in-place updates;
// e.g. the MS controller will give highest delete priority to Machines still updating in-place,
// which are also unavailable Machines.
//
// Notably, there is also no agreement yet on a different way forward because e.g. limiting scale down of the
// new MS could lead e.g. to completing in-place update of Machines that will be otherwise deleted.
func (p *rolloutPlanner) reconcileNewMachineSet(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(p.oldMSs, p.newMS)

	if *(p.newMS.Spec.Replicas) == *(p.md.Spec.Replicas) {
		// Scaling not required.
		return nil
	}

	if *(p.newMS.Spec.Replicas) > *(p.md.Spec.Replicas) {
		// Scale down.
		p.addNotef(p.newMS, "scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas")
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas", klog.KObj(p.newMS), *(p.md.Spec.Replicas)), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = *(p.md.Spec.Replicas)
		return nil
	}

	newReplicasCount, note, err := mdutil.NewMSNewReplicas(p.md, allMSs, *p.newMS.Spec.Replicas)
	if err != nil {
		return err
	}

	if newReplicasCount < *(p.newMS.Spec.Replicas) {
		scaleDownCount := *(p.newMS.Spec.Replicas) - newReplicasCount
		p.addNotef(p.newMS, "%s", note)
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", klog.KObj(p.newMS), newReplicasCount, scaleDownCount), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = newReplicasCount
	}
	if newReplicasCount > *(p.newMS.Spec.Replicas) {
		scaleUpCount := newReplicasCount - *(p.newMS.Spec.Replicas)
		p.addNotef(p.newMS, "%s", note)
		log.V(5).Info(fmt.Sprintf("Setting scale up intent for MachineSet %s to %d replicas (+%d)", klog.KObj(p.newMS), newReplicasCount, scaleUpCount), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = newReplicasCount
	}
	return nil
}

func (p *rolloutPlanner) reconcileOldMachineSetsRollingUpdate(ctx context.Context) error {
	allMSs := append(p.oldMSs, p.newMS)

	// no op if there are no replicas on old machinesets
	if mdutil.GetReplicaCountForMachineSets(p.oldMSs) == 0 {
		return nil
	}

	maxUnavailable := mdutil.MaxUnavailable(*p.md)
	minAvailable := max(ptr.Deref(p.md.Spec.Replicas, 0)-maxUnavailable, 0)

	totalSpecReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)

	// Find the total number of available replicas.
	totalAvailableReplicas := ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(allMSs), 0)

	// Find the number of pending scale down from previous reconcile/from current reconcile.
	// This is required because whenever the system is reducing the number of replicas, this operation could further impact availability e.g.
	// - in case of regular rollouts, there is no certainty about which replica is going to be deleted (and if the replica being deleted is currently available or not):
	// 	 - e.g. MS controller is going to first delete replicas with deletion annotation; also MS controller has a slightly different notion of unavailable as of now.
	// - in case of in-place rollout, in-place updates are always assumed as impacting availability (they can always fail).
	totalPendingScaleDown := int32(0)
	for _, ms := range allMSs {
		scaleIntent := ptr.Deref(ms.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[ms.Name]; ok {
			scaleIntent = min(scaleIntent, v)
		}

		// NOTE: Count only pending scale down from the current status.replicas (so scale down of existing replicas).
		if scaleIntent < ptr.Deref(ms.Status.Replicas, 0) {
			totalPendingScaleDown += max(ptr.Deref(ms.Status.Replicas, 0)-scaleIntent, 0)
		}
	}

	// Consider if there are machine updating in-place where the operation was declared as not affecting availability.
	//
	// Even if this is not impacting availability, it is required to consider those machines to slow down rollout/
	// ensure that changes are rolled out in an incremental way.
	// Note: this is consistent with the idea that maxSurge and maxUnavailable defines the speed of a rollout (with the
	// caveat that maxSurge/creating additional machines is capped because rollout planner must give priority to in-place when possible).
	// Note: Machines InPlaceUpdatingNotAffectingAvailability exists on oldMSs only if another change is applied
	// while in place update is still in progress.
	totalInPlaceUpdatingNotAffectingAvailability := int32(0)
	for _, m := range p.machines {
		if inplace.IsUpdateInProgressNotAffectingAvailability(m) {
			totalInPlaceUpdatingNotAffectingAvailability++
		}
	}

	// On top of considering maxUnavailable, before scaling down it is also important to consider how unavailability is distributed
	// across MachineSets, because if there are unavailable machines on the new MachineSet it is necessary to slow down or stop rollout
	// to prevent to transition all the Machines to a broken state.
	// In order to do so, compute the number of unavailable machines on the new MachineSet and use it to reduce totalScaleDownCount.
	newMSUnavailableMachineCount := max(ptr.Deref(p.newMS.Spec.Replicas, 0)-ptr.Deref(p.newMS.Status.AvailableReplicas, 0), 0)

	// Compute the total number of replicas that can be scaled down.
	// Exit immediately if there is no room for scaling down.
	// NOTE: this is a quick preliminary check to verify if there is room for scaling down any of the oldMSs; further down the code
	// will make additional checks to ensure scale down actually happens without breaching MaxUnavailable, and
	// if necessary, it will reduce the extent of the scale down accordingly.
	totalScaleDownCount := max(totalSpecReplicas-totalPendingScaleDown-totalInPlaceUpdatingNotAffectingAvailability-minAvailable-newMSUnavailableMachineCount, 0)
	if totalScaleDownCount <= 0 {
		return nil
	}

	// Sort oldMSs so the system will start deleting from the oldest MS first.
	sort.Sort(mdutil.MachineSetsByCreationTimestamp(p.oldMSs))

	// Scale down only unavailable replicas / up to residual totalScaleDownCount.
	// NOTE: The system must scale down unavailable replicas first in order to increase chances for the rollout to progress.
	// However, rollout planner must also take into account the fact that the MS controller might have a different opinion on
	// which replica to delete when a scale down happens.
	//
	// As a consequence, the rollout planner cannot assume a scale down operation deletes an unavailable replica. e.g.
	//  - MD with spec.replicas 3, MaxSurge 1, MaxUnavailable 0
	//  - OldMS with 3 replicas, 2 available replica (and thus 1 unavailable replica)
	//  - NewMS with 1 replica, 1 available replica
	//  - In theory it is possible to scale down oldMS from 3->2 replicas by deleting the unavailable replica.
	//  - However,  rollout planner cannot assume that the MachineSet controller is going to delete
	//    the unavailable replica when scaling down from 3->2, because it might happen that one of the available replicas
	//    is deleted instead.
	//
	// In the example above, the scaleDownOldMSs should not scale down OldMS from 3->2 replicas.
	// In other use cases, e.g. when scaling down an oldMS from 5->3 replicas, the scaleDownOldMSs should limit scale down extent
	// only partially if this doesn't breach MaxUnavailable (e.g. scale down 5->4 instead of 5->3).
	totalScaleDownCount, totalAvailableReplicas = p.scaleDownOldMSs(ctx, totalScaleDownCount, totalAvailableReplicas, minAvailable, true)

	// Then scale down old MS down to zero replicas / down to residual totalScaleDownCount.
	// NOTE: Also in this case, we should continuously assess if reducing the number of replicase could further impact availability,
	// and if necessary, limit scale down extent to ensure the operation respects MaxUnavailable limits.
	_, _ = p.scaleDownOldMSs(ctx, totalScaleDownCount, totalAvailableReplicas, minAvailable, false)

	return nil
}

func (p *rolloutPlanner) scaleDownOldMSs(ctx context.Context, totalScaleDownCount, totalAvailableReplicas, minAvailable int32, scaleDownOnlyUnavailableReplicas bool) (int32, int32) {
	log := ctrl.LoggerFrom(ctx)

	for _, oldMS := range p.oldMSs {
		// No op if there is no scaling down left.
		if totalScaleDownCount <= 0 {
			break
		}

		replicas := ptr.Deref(oldMS.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[oldMS.Name]; ok {
			replicas = v
		}

		// No op if this MS has been already scaled down to zero.
		if replicas <= 0 {
			continue
		}

		oldTotalAvailableReplicas := totalAvailableReplicas

		// Compute the scale down extent by considering either all replicas or, if scaleDownOnlyUnavailableReplicas is set, unavailable replicas only.
		// In both cases, scale down is limited to totalScaleDownCount.
		// Exit if there is no room for scaling down the MS.
		// NOTE: this is a preliminary check to verify if there is room for scaling down this specific MS; further down the code
		// will make additional checks to ensure scale down actually happens without breaching MaxUnavailable, and
		// if necessary, it will reduce the extent of the scale down accordingly.
		maxScaleDown := replicas
		if scaleDownOnlyUnavailableReplicas {
			maxScaleDown = max(replicas-ptr.Deref(oldMS.Status.AvailableReplicas, 0), 0)
		}
		scaleDown := min(maxScaleDown, totalScaleDownCount)
		if scaleDown == 0 {
			continue
		}

		// Before scaling down validate if the operation will lead to a breach of minAvailability.
		// In order to do so, consider how many existing replicas will be actually deleted, and consider
		// this operation as impacting availability because there are no guarantees that the MS controller is going to
		// delete unavailable replicas first; if the projected state breaches minAvailability, reduce the scale down extend accordingly.

		// If there are no available replicas on this MS, scale down won't impact totalAvailableReplicas at all.
		if ptr.Deref(oldMS.Status.AvailableReplicas, 0) > 0 {
			// If instead there are AvailableReplicas on this MS:
			// compute the new spec.replicas assuming we are going to use the entire scale down extent.
			newSpecReplicas := max(replicas-scaleDown, 0)

			// compute how many existing replicas the operation is going to delete:
			// e.g. if MS is scaling down spec.replicas from 5 to 3, but status.replicas is 4, it is scaling down 1 existing replica.
			// e.g. if MS is scaling down spec.replicas from 5 to 3, but status.replicas is 6, it is scaling down 3 existing replicas.
			existingReplicasToBeDeleted := max(ptr.Deref(oldMS.Status.Replicas, 0)-newSpecReplicas, 0)

			// If we are deleting at least one existing replicas
			if existingReplicasToBeDeleted > 0 {
				// Check if we are scaling down more existing replica than what is allowed by MaxUnavailability.
				if totalAvailableReplicas-minAvailable < existingReplicasToBeDeleted {
					// If we are scaling down more existing replica than what is allowed by MaxUnavailability, then
					// rollout planner must revisit the scale down extent.

					// Determine how many replicas can be deleted overall without further impacting availability.
					maxExistingReplicasThatCanBeDeleted := max(totalAvailableReplicas-minAvailable, 0)

					// Compute the revisited new spec.replicas:
					// e.g. MS spec.replicas 20, scale down 8, newSpecReplicas 12 (20-8), status.replicas 15 -> existingReplicasToBeDeleted 3 (15-12)
					//   assuming that maxExistingReplicasThatCanBeDeleted is 2, newSpecReplicasRevisited should be 15-2 = 13
					// e.g. MS spec.replicas 16, scale down 3, newSpecReplicas 13 (16-3), status.replicas 21 -> existingReplicasToBeDeleted 8 (21-13)
					//   assuming that maxExistingReplicasThatCanBeDeleted is 7, newSpecReplicasRevisited should be 21-7 = 14
					// NOTE: there is a safeguard preventing to go above the initial replicas number (this is scale down oldMS).
					newSpecReplicasRevisited := min(ptr.Deref(oldMS.Status.Replicas, 0)-maxExistingReplicasThatCanBeDeleted, replicas)

					// Re-compute the scale down extent by using newSpecReplicasRevisited.
					scaleDown = max(replicas-newSpecReplicasRevisited, 0)

					// Re-compute how many existing replicas the operation is going to delete by using newSpecReplicasRevisited.
					existingReplicasToBeDeleted = max(ptr.Deref(oldMS.Status.Replicas, 0)-newSpecReplicasRevisited, 0)
				}

				// keep track that we are deleting existing replicas by assuming that this operation
				// will reduce totalAvailableReplicas (worst scenario, deletion of available machines happen first).
				totalAvailableReplicas -= min(ptr.Deref(oldMS.Status.AvailableReplicas, 0), existingReplicasToBeDeleted)
			}
		}

		if scaleDown > 0 {
			newScaleIntent := max(replicas-scaleDown, 0)
			// Check if totalAvailableReplicas has been changed above and use it as a signal to determine if scale down
			// was to align to existing Machines (replicas count > number of machines) or if scale down was performed
			// removing unavailable replicas when no available replicas exist on the MachineSet.
			// Note. In both cases overall availability is not impacted.
			if oldTotalAvailableReplicas == totalAvailableReplicas {
				p.addNotef(oldMS, "scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)")
			} else {
				p.addNotef(oldMS, "%d available replicas > %d minimum available replicas", oldTotalAvailableReplicas, minAvailable)
			}
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", klog.KObj(oldMS), newScaleIntent, scaleDown), "MachineSet", klog.KObj(oldMS))
			p.scaleIntents[oldMS.Name] = newScaleIntent
			totalScaleDownCount = max(totalScaleDownCount-scaleDown, 0)
		}
	}

	return totalScaleDownCount, totalAvailableReplicas
}

// oldMSRolloutDecision is the decision about how to roll out changes for the Machines of an oldMS.
type oldMSRolloutDecision string

const (
	// oldMSRolloutPendingDecision means that the extension asked to be called again later before taking a decision.
	oldMSRolloutPendingDecision oldMSRolloutDecision = "Pending"
	// oldMSRolloutUpdateInPlaceDecision means that Machines are going to be moved to the newMS and updated in-place.
	oldMSRolloutUpdateInPlaceDecision oldMSRolloutDecision = "UpdateInPlace"
	// oldMSRolloutReplaceDecision means that Machines are going to be replaced.
	// Note: This decision also applies when in-place update is possible but not allowed or when in-place update is not possible (fallback).
	oldMSRolloutReplaceDecision oldMSRolloutDecision = "Replace"
	// oldMSRolloutBlockedDecision means that the extension does not allow any operation that can roll out the changes; as a consequence, the rollout is blocked.
	oldMSRolloutBlockedDecision oldMSRolloutDecision = "Blocked"
)

// rolloutDecision returns how to roll out changes for the oldMS, considering that allowedOperations is orthogonal to
// the fact that the extension can cover the entire diff with patches.
func rolloutDecision(r canUpdateMachineSetResult) oldMSRolloutDecision {
	switch {
	case r.retryAfterFromExtension:
		return oldMSRolloutPendingDecision
	case r.canUpdateMachineSet && isOperationAllowed(r.allowedOperations, runtimehooksv1.MachineUpdateOperationUpdateInPlace):
		return oldMSRolloutUpdateInPlaceDecision
	case isOperationAllowed(r.allowedOperations, runtimehooksv1.MachineUpdateOperationReplace):
		return oldMSRolloutReplaceDecision
	default:
		return oldMSRolloutBlockedDecision
	}
}

// reconcileInPlaceUpdateIntent ensures CAPI rolls out changes by performing in-place updates whenever possible,
// and it enforces the binding opinion of the CanUpdateMachineSet extension about how to roll out changes.
//
// When calling this func, new and old MS already have their scale intent, which was computed under the assumption that
// rollout is going to happen by delete/re-create, and thus it will impact availability.
//
// By default in-place updates are assumed to impact availability because the system must account
// for scenarios when the operation fails, leading to remediation of the machine/unavailability.
//
// First, this func determines a decision for each oldMS with replicas:
//   - Pending: the extension asked to be called again later.
//   - UpdateInPlace: in-place update operation is possible and allowed; the oldMS moves Machines to the newMS instead of deleting them.
//   - Replace: in-place update operation is not possible or not allowed, and replace operation is allowed (also as a fallback).
//   - Blocked: no allowed operation can roll out changes; the scale intent for the oldMS is dropped.
//
// Then:
//   - If at least one decision is Pending, the entire rollout is paused because it is not possible to plan how to optimize for in-place
//     without all the answers.
//     As a consequence: scale intents of all the oldMSs and the part of the scale up
//     intent of the newMS that uses maxSurge are dropped. Scale up/down of the newMS due to changes to md.spec.replicas are preserved.
//     Note: OldMSs with an InPlaceUpdate decision still get the move annotations, so moves that are already in progress are not
//     turned into deletions.
//   - The newMS has no CanUpdateMachineSet answer, so the usage of maxSurge is attributed to the operations performed on the oldMSs:
//     if at least one oldMS is going to be updated in-place maxSurge is considered part of the in-place update operation,
//     and this is consistent with the requirement to prioritize in-place when possible;
//     otherwise, if no OldMS is going to be updated in-place and at least one oldMS is replacing machines (including fallbacks from in-place)
//     maxSurge is considered part of the replace operation; if all the oldMSs are blocked, maxSurge is blocked as well.
//
// Note:
//   - This function can rely on scale intent previously computed, and in the case in-place update is possible,
//     influence how scale down on oldMS is performed.
//   - In case of update in-place, creation of additional machines due to maxSurge is capped to 1 or entirely dropped because
//     rollout planner must give priority to in-place when possible.
//     More specifically, using one slot for maxSurge is required to create an additional machine at the
//     beginning of an in-place upgrade unless MaxUnavailable >= 1 (the MD configuration allows introducing unavailability)
//     or the runtime extension declares that it can perform the in-place update without affecting availability.
//   - Creation of new machines due to scale up goes through as usual.
//
// Warning! When the runtime extension declares that it can perform the in-place update without affecting availability,
// then Cluster API does not provide the safety net that is provided by creating additional machines.
// In case of problems during such in-place updates, user workloads might be impacted and/or disrupted.
func (p *rolloutPlanner) reconcileInPlaceUpdateIntent(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return nil
	}

	// If new MS already has all desired replicas, it does not make sense to perform more in-place updates.
	if ptr.Deref(p.newMS.Spec.Replicas, 0) >= ptr.Deref(p.md.Spec.Replicas, 0) {
		return nil
	}

	// Determine the rollout decision for each oldMS.
	inPlaceUpdateCandidates := sets.Set[string]{}
	inPlaceUpdateCandidatesNotAffectingAvailability := []*clusterv1.MachineSet{}
	replaceOperationMSs := []*clusterv1.MachineSet{}
	for _, oldMS := range p.oldMSs {
		// If the oldMS doesn't have replicas anymore, nothing left to do.
		if ptr.Deref(oldMS.Status.Replicas, 0) <= 0 {
			continue
		}

		// If the oldMS is not eligible for in-place updates, changes can be rolled out only by replacing machines.
		// Note: The extension is not called in this case, so it cannot block the rollout.
		if result, ok := p.upToDateResults[oldMS.Name]; !ok || !result.EligibleForInPlaceUpdate {
			replaceOperationMSs = append(replaceOperationMSs, oldMS)
			continue
		}

		// Check if the MachineSet can update in place and which operations are allowed.
		res, err := p.canUpdateMachineSetInPlace(ctx, oldMS, p.newMS)
		if err != nil {
			// Note: internalErrorMessage is only set if calling the extensions failed, other errors are not caused by the hook.
			if res.internalErrorMessage != "" {
				p.canUpdateMachineSetResults.internalErrorMessage = canUpdateMachineSetHookMessage(oldMS, res.internalErrorMessage)
			}
			return pkgerrors.Wrapf(err, "failed to determine if MachineSet %s can be updated in-place", oldMS.Name)
		}
		decision := rolloutDecision(res)
		log.V(5).Info(fmt.Sprintf("CanUpdate decision for MachineSet %s: %s, can update in-place: %t, affects availability: %t", klog.KObj(oldMS), decision, res.canUpdateMachineSet, res.affectsAvailability), "MachineSet", klog.KObj(oldMS))

		switch decision {
		case oldMSRolloutPendingDecision:
			p.rolloutDecisionPending = true
			// Note: The rollout stays paused until all the pending oldMSs get a decision, and cached pending results are used until
			// their retryAfter, so the pause cannot be lifted before the latest retryAfter.
			if res.retryAfter.After(p.pendingRetryAfter) {
				p.pendingRetryAfter = res.retryAfter
			}
			p.canUpdateMachineSetResults.waitingForDecisionMessages = append(p.canUpdateMachineSetResults.waitingForDecisionMessages, canUpdateMachineSetHookMessage(oldMS, strings.Join(res.messages, ", ")))
			p.addNotef(oldMS, "rollout paused, waiting for the extension to take a decision about how to roll out changes (retry after %s)", res.retryAfter.Sub(p.now()).Truncate(time.Second))
		case oldMSRolloutBlockedDecision:
			if p.blockedMSs == nil {
				p.blockedMSs = sets.Set[string]{}
			}
			p.blockedMSs.Insert(oldMS.Name)
			// Note: Blocked oldMSs are re-evaluated independently, so we have to reconcile when the first cached result expires.
			if p.blockedRetryAfter.IsZero() || res.retryAfter.Before(p.blockedRetryAfter) {
				p.blockedRetryAfter = res.retryAfter
			}
			// Note: The decision is only blocked if the Replace operation is not allowed.
			var msg string
			if !isOperationAllowed(res.allowedOperations, runtimehooksv1.MachineUpdateOperationUpdateInPlace) {
				msg = "neither replace nor in-place update operations are allowed"
			} else {
				msg = "replace operation is not allowed"
				// Note: In MD, affecting availability never blocks the rollout, it only affects the decision if
				// maxSurge should be used or converted to a scale down (see below in this func).
				if !res.canUpdateMachineSet {
					msg += " and not all changes can be updated in-place"
				}
			}
			if len(res.messages) > 0 {
				msg += fmt.Sprintf(": %s", strings.Join(res.messages, ", "))
			}
			p.canUpdateMachineSetResults.rolloutBlockedMessages = append(p.canUpdateMachineSetResults.rolloutBlockedMessages, canUpdateMachineSetHookMessage(oldMS, msg))
			p.addNotef(oldMS, "rollout blocked, %s", msg)
			log.V(5).Info(fmt.Sprintf("MachineSet %s cannot be rolled out: rollout is blocked by CanUpdateMachineSet hook: %s", klog.KObj(oldMS), msg), "MachineSet", klog.KObj(oldMS))
		case oldMSRolloutReplaceDecision:
			replaceOperationMSs = append(replaceOperationMSs, oldMS)
			if res.canUpdateMachineSet {
				p.addNotef(oldMS, "in-place update is possible but not allowed, falling back to replace machines")
			}
		case oldMSRolloutUpdateInPlaceDecision:
			// Set the annotation informing the oldMS that it must move machines to the newMS instead of deleting them.
			// Note: After a machine is moved from oldMS to newMS, the newMS will take care of the in-place update process.
			// Note: Cleanup of the MachineSetMoveMachinesToMachineSetAnnotation will happen automatically as soon as the rollout planner stops
			// to set it, because this annotation is not part of the output of computeDesiredMS
			// (same applies to newMS, so annotation will always be removed from newMS).
			// Note: The annotation is set also when the rollout is paused (pending decision for another oldMS), so
			// scale downs already in progress keep moving Machines instead of deleting them.
			if oldMS.Annotations == nil {
				oldMS.Annotations = map[string]string{}
			}

			moveMachinesToMachineSetAnnotationData := clusterv1.MachineSetMoveMachinesToMachineSetAnnotationData{
				Name:                p.newMS.Name,
				AffectsAvailability: new(res.affectsAvailability),
			}
			if res.affectsAvailability {
				p.addNotef(oldMS, "should scale down by moving Machines to MachineSet %s (affects availability)", p.newMS.Name)
			} else {
				inPlaceUpdateCandidatesNotAffectingAvailability = append(inPlaceUpdateCandidatesNotAffectingAvailability, oldMS)
				p.addNotef(oldMS, "should scale down by moving Machines to MachineSet %s (does not affect availability)", p.newMS.Name)
			}
			dataBytes, err := json.Marshal(moveMachinesToMachineSetAnnotationData)
			if err != nil {
				return pkgerrors.Wrapf(err, "failed to trigger in-place update for MachineSet %s by setting the %s annotation: failed to Marshal data", klog.KObj(oldMS), clusterv1.MachineSetMoveMachinesToMachineSetAnnotation)
			}
			oldMS.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation] = string(dataBytes)
			inPlaceUpdateCandidates.Insert(oldMS.Name)
		}
	}

	// Set the annotation informing the newMS that it will receive replicas moved from oldMS selected as in-place candidates.
	// Note: there is a two-ways check before the move operation:
	// 	"oldMS must have: move to newMS" and "newMS must have: accept replicas from oldMS"
	// Note: Cleanup of the MachineSetReceiveMachinesFromMachineSetsAnnotation will happen automatically as soon as the rollout planner stops
	// to set it, because this annotation is not part of the output of computeDesiredMS
	// (same applies to oldMS, so annotation will always be removed from oldMS).
	if inPlaceUpdateCandidates.Len() > 0 {
		if p.newMS.Annotations == nil {
			p.newMS.Annotations = map[string]string{}
		}
		p.newMS.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation] = sortAndJoin(inPlaceUpdateCandidates.UnsortedList())
	}

	// Compute how much of the scale up intent of the newMS is using maxSurge.
	// Note: This must happen before dropping scale intents of oldMSs, because the computation is based on the intents set so far.
	scaleUpCount, maxSurgeUsed := p.newMSMaxSurgeUsed()

	// If at least one oldMS is waiting for a decision from the extension, pause the rollout.
	// Pausing the rollout implies dropping scale down intent from oldMSs and dropping maxSurge for the newMS scale up intent (scale up for other reasons is preserved).
	if p.rolloutDecisionPending {
		for _, oldMS := range p.oldMSs {
			if _, ok := p.scaleIntents[oldMS.Name]; ok {
				log.V(5).Info(fmt.Sprintf("Dropping scale intent for MachineSet %s while waiting for the extension to take a decision about how to roll out changes", klog.KObj(oldMS)), "MachineSet", klog.KObj(oldMS))
				delete(p.scaleIntents, oldMS.Name)
			}
		}
		p.dropNewMSMaxSurge(ctx, scaleUpCount, maxSurgeUsed, "rollout paused, waiting for the extension to take a decision about how to roll out changes")
		return nil
	}

	// If rollout is not paused due to retry after, rollout might be blocked for some of the OldMSs.
	// Drop scale intent for oldMSs that are blocked.
	// Note: We iterate over p.oldMSs instead of p.blockedMSs because blockedMSs only has names while logging requires the MachineSet.
	for _, oldMS := range p.oldMSs {
		if !p.blockedMSs.Has(oldMS.Name) {
			continue
		}
		if _, ok := p.scaleIntents[oldMS.Name]; ok {
			log.V(5).Info(fmt.Sprintf("Dropping scale intent for MachineSet %s because the rollout is blocked", klog.KObj(oldMS)), "MachineSet", klog.KObj(oldMS))
			delete(p.scaleIntents, oldMS.Name)
		}
	}

	// Everything below this point is about checking if rollout planner is using maxSurge,
	// and if possible, drop the usage of maxSurge / minimize unnecessary Machine creations because rollout planner
	// must give priority to in-place when possible.

	// If the newMS is not scaling up using maxSurge, nothing left to do (we have to create new machines).
	if maxSurgeUsed <= 0 {
		return nil
	}

	// If there are no oldMSs to be updated in-place, the usage of maxSurge can only be considered part of a replace machine operation.
	if inPlaceUpdateCandidates.Len() <= 0 {
		// When part of a replace machine operation, maxSurge should be blocked if
		// there is a chance we want to use maxSurge for this operation when it will unblock,
		// and it is not required to unblock replace operations.
		maxSurgeIsBlocked := len(replaceOperationMSs) == 0 && p.blockedMSs.Len() > 0
		if maxSurgeIsBlocked {
			p.dropNewMSMaxSurge(ctx, scaleUpCount, maxSurgeUsed, "rollout blocked, replacing machines is not allowed")
		}
		return nil
	}

	// At this point, rollout planner know that scale down of at least one OldMS is going to happen using move and the update in-place operation.
	// In this case, rollout planner must prioritize in-place updates over creation of new Machines.
	// As a consequence, it is required to revisit the scale up intent for the newMS and defer as much as possible
	// of the creation of new Machines due to maxSurge.
	newScaleUpCount := max(scaleUpCount-maxSurgeUsed, 0)

	// If the revisited scale up count for the newMS is 0 and the newMS is not already scaling up from a previous reconcile (no scale up at all),
	// check if the rollout planner is required to use one slot from MaxSurge e.g. to start or unblock a rollout.
	// Note: Rollout planner will use one slot from MaxSurge - one and not more - only if:
	// - The rollout is not progressing in other ways (on top of newMS not scaling from a previous reconcile, there are also no oldMS scaling down)
	// - There are no in-place updates in progress (the rollout planner must wait for in-place updates to complete or fail/go
	//   through remediation before creating additional machines)
	if newScaleUpCount == 0 && !p.scalingOrInPlaceUpdateInProgress(ctx) {
		newScaleUpCount = 1
	} else {
		p.addNotef(p.newMS, "surge %d dropped to prioritize in-place updates", maxSurgeUsed)

		newScaleIntent := ptr.Deref(p.newMS.Spec.Replicas, 0) + newScaleUpCount
		log.V(5).Info(fmt.Sprintf("Revisited scale up intent for MachineSet %s to %d replicas (+%d) to prevent creation of new machines while there are still in-place updates to be performed", klog.KObj(p.newMS), newScaleIntent, newScaleUpCount), "MachineSet", klog.KObj(p.newMS))
		if newScaleUpCount == 0 {
			delete(p.scaleIntents, p.newMS.Name)
		} else {
			p.scaleIntents[p.newMS.Name] = newScaleIntent
		}
		return nil
	}

	// At the point, we know that rollout is not progressing in other ways (on top of newMS not scaling from a previous reconcile, there are also no oldMS scaling down
	// and no in-place upgrade in progress), so it looks like the only way to progress is to scale up newMS by one by using one slot from maxSurge.
	//
	// Make a final try to check if we can avoid using maxSurge / scale up newMS by one.
	//
	// If at least one of the oldMS candidate for in-place update can perform an update without affecting availability,
	// then we have a way forward that will not impact availability and that abides to the principle of prioritizing in-place updates over creation of new Machines.
	if len(inPlaceUpdateCandidatesNotAffectingAvailability) > 0 {
		// Drop scale up newMS / usage of MaxSurge
		newScaleUpCount = 0

		// Scale down one of the oldMS candidate for in-place update and that can perform an update without affecting availability.
		// Sort oldMSs so the system will start moving machines from the oldest MS first.
		sort.Sort(mdutil.MachineSetsByCreationTimestamp(inPlaceUpdateCandidatesNotAffectingAvailability))
		oldMS := inPlaceUpdateCandidatesNotAffectingAvailability[0]

		replicasOldMS := ptr.Deref(oldMS.Spec.Replicas, 0)
		if i, ok := p.scaleIntents[oldMS.Name]; ok {
			replicasOldMS = i
		}
		newScaleDownIntentOldMS := max(replicasOldMS-1, 0)
		p.addNotef(oldMS, "surge %d for MachineSet %s converted to scale down for MachineSet %s to %d replicas (-%d)", maxSurgeUsed, p.newMS.Name, oldMS.Name, newScaleDownIntentOldMS, 1)
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", klog.KObj(oldMS), newScaleDownIntentOldMS, 1), "MachineSet", klog.KObj(oldMS))
		p.scaleIntents[oldMS.Name] = newScaleDownIntentOldMS
	} else {
		p.addNotef(p.newMS, "surge 1 allowed to create availability for in-place updates")
	}

	newScaleIntent := ptr.Deref(p.newMS.Spec.Replicas, 0) + newScaleUpCount
	log.V(5).Info(fmt.Sprintf("Revisited scale up intent for MachineSet %s to %d replicas (+%d) to prevent creation of new machines while there are still in-place updates to be performed", klog.KObj(p.newMS), newScaleIntent, newScaleUpCount), "MachineSet", klog.KObj(p.newMS))
	if newScaleUpCount == 0 {
		delete(p.scaleIntents, p.newMS.Name)
	} else {
		p.scaleIntents[p.newMS.Name] = newScaleIntent
	}

	return nil
}

func canUpdateMachineSetHookMessage(oldMS *clusterv1.MachineSet, details string) string {
	msg := fmt.Sprintf("Blocked by CanUpdateMachineSet hook for MachineSet %s", oldMS.Name)
	if details != "" {
		msg = fmt.Sprintf("%s: %s", msg, details)
	}
	return msg
}

// newMSMaxSurgeUsed returns the size of the current scale up intent for the newMS and how much of it is using maxSurge.
//
// We consider that scale up intent is using maxSurge when the target total number of replicas will be greater than md.spec replicas,
// more specifically:
//
// The target total number of replicas is computed considering existing replicas and scale intents (it mimics the computation
// in mdutil.TotalMachineSetsReplicaSum +/- scale intent).
//
// If the target total number of replicas is greater than md.spec.replicas, it means that there will be exceeding replicas,
// due to one of the following reasons:
//   - reconcileNewMachineSet used maxSurge when setting the scale intent for the newMS (note that reconcileNewMachineSet can also change set the scale intent for the newMS the user scaled up/down md.spec.replicas).
//   - The user scaled down md.spec.replicas and exceeding replicas are not yet deleted.
//
// The actual amount of maxSurge slots used can be identified by looking at the number of replicas
// being added to the new MS, and picking the min between this value and the number of exceeding replicas.
func (p *rolloutPlanner) newMSMaxSurgeUsed() (scaleUpCount, maxSurgeUsed int32) {
	// If the newMS is not scaling up, there is no maxSurge in use.
	if scaleIntent, ok := p.scaleIntents[p.newMS.Name]; !ok || scaleIntent <= ptr.Deref(p.newMS.Spec.Replicas, 0) {
		return 0, 0
	}

	totalReplicas := int32(0)
	for _, ms := range append(p.oldMSs, p.newMS) {
		if ms != nil {
			replicas := ptr.Deref(ms.Spec.Replicas, 0)
			if scaleIntent, ok := p.scaleIntents[ms.Name]; ok {
				replicas = scaleIntent
			}
			totalReplicas += max(replicas, ptr.Deref(ms.Status.Replicas, 0))
		}
	}
	exceedingReplicas := max(totalReplicas-ptr.Deref(p.md.Spec.Replicas, 0), 0)

	scaleUpCount = max(p.scaleIntents[p.newMS.Name]-ptr.Deref(p.newMS.Spec.Replicas, 0), 0)
	return scaleUpCount, min(scaleUpCount, exceedingReplicas)
}

// dropNewMSMaxSurge removes from the scale up intent of the newMS the replicas that are using maxSurge.
// Note: The part of the scale up intent that is not using maxSurge, e.g. due to md.spec.replicas being scaled up, is preserved.
func (p *rolloutPlanner) dropNewMSMaxSurge(ctx context.Context, scaleUpCount, maxSurgeUsed int32, reason string) {
	log := ctrl.LoggerFrom(ctx)

	if maxSurgeUsed <= 0 {
		return
	}

	newScaleUpCount := max(scaleUpCount-maxSurgeUsed, 0)
	newScaleIntent := ptr.Deref(p.newMS.Spec.Replicas, 0) + newScaleUpCount
	p.addNotef(p.newMS, "surge %d dropped: %s", maxSurgeUsed, reason)
	log.V(5).Info(fmt.Sprintf("Revisited scale up intent for MachineSet %s to %d replicas (+%d) because %s", klog.KObj(p.newMS), newScaleIntent, newScaleUpCount, reason), "MachineSet", klog.KObj(p.newMS))
	if newScaleUpCount == 0 {
		delete(p.scaleIntents, p.newMS.Name)
	} else {
		p.scaleIntents[p.newMS.Name] = newScaleIntent
	}
}

func (p *rolloutPlanner) scalingOrInPlaceUpdateInProgress(_ context.Context) bool {
	// Check if the new MS or old MS are scaling.
	if ptr.Deref(p.newMS.Spec.Replicas, 0) != ptr.Deref(p.newMS.Status.Replicas, 0) {
		return true
	}
	for _, oldMS := range p.oldMSs {
		if ptr.Deref(oldMS.Spec.Replicas, 0) < ptr.Deref(oldMS.Status.Replicas, 0) {
			return true
		}
		if scaleIntent, ok := p.scaleIntents[oldMS.Name]; ok && scaleIntent < ptr.Deref(oldMS.Spec.Replicas, 0) {
			return true
		}
	}

	// Check that there are no updates in progress.
	// We check both that the newMS MachineSet will report that the update is completed via .status.upToDateReplicas
	// and the Machine controller through the annotations so this code does not depend on a specific execution order
	// of the MachineSet and Machine controllers.
	if ptr.Deref(p.newMS.Spec.Replicas, 0) != ptr.Deref(p.newMS.Status.UpToDateReplicas, 0) {
		return true
	}
	for _, m := range p.machines {
		if inplace.IsUpdateInProgress(m) {
			p.updatingMachineNames = append(p.updatingMachineNames, m.Name)
		}
	}
	if len(p.updatingMachineNames) > 0 {
		return true
	}

	// We are also checking AvailableReplicas because we want to make sure that we wait until the
	// Machine goes back to Available after the in-place update is completed.
	// If we would not wait for this, the rolloutPlaner would use maxSurge to create an additional Machine.
	// Note: This also means that if any Machine of the new MachineSet becomes unavailable we are blocking
	// further progress of the in-place update.
	if ptr.Deref(p.newMS.Spec.Replicas, 0) != ptr.Deref(p.newMS.Status.AvailableReplicas, 0) {
		return true
	}
	return false
}

// This funcs tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
// Note: This func must be called after computing scale up/down intent for all the MachineSets.
// Note: This func only address deadlock due to unavailable machines not getting deleted on oldMSs, e.g. due to a wrong configuration.
// Note: Unblocking deadlocks when unavailable replicas exist only on oldMSs, is required also because replicas on oldMSs are not remediated by MHC.
func (p *rolloutPlanner) reconcileDeadlockBreaker(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(p.oldMSs, p.newMS)

	// If the rollout is paused waiting for a decision from the extension, or blocked because no allowed operation can roll out changes,
	// this is not a deadlock, it is the expected state.
	if p.rolloutDecisionPending || p.blockedMSs.Len() > 0 {
		return
	}

	// if there are no replicas on the old MS, rollout is completed, no deadlock (actually no rollout in progress).
	if ptr.Deref(mdutil.GetActualReplicaCountForMachineSets(p.oldMSs), 0) == 0 {
		return
	}

	// if all the replicas on OldMS are available, no deadlock (regular scale up newMS and scale down oldMS should take over from here).
	if ptr.Deref(mdutil.GetActualReplicaCountForMachineSets(p.oldMSs), 0) == ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(p.oldMSs), 0) {
		return
	}

	// If there are scale operation in progress, no deadlock.
	// Note: we are considering both scale operation from previous and current reconcile.
	for _, ms := range allMSs {
		if ptr.Deref(ms.Spec.Replicas, 0) != ptr.Deref(ms.Status.Replicas, 0) {
			return
		}
		if _, ok := p.scaleIntents[ms.Name]; ok {
			return
		}
	}

	// if there are unavailable replicas on the newMS, wait for them to become available first.
	// Note: A rollout cannot be unblocked if new machines do not become available.
	// Note: If the replicas on the newMS are not becoming available either:
	// - automatic remediation can help in addressing temporary failures.
	// - user intervention is required to fix more permanent issues e.g. to fix a wrong configuration.
	if ptr.Deref(p.newMS.Status.AvailableReplicas, 0) != ptr.Deref(p.newMS.Status.Replicas, 0) {
		return
	}

	// At this point we can assume there is a deadlock that can only be remediated by breaching maxUnavailability constraint
	// and scaling down an oldMS with unavailable machines by one.
	//
	// Note: In most cases this is only a formal violation of maxUnavailability, because there is a good chance
	// that the machine that will be deleted is one of the unavailable machines.
	// Note: This for loop relies on the same ordering of oldMSs that has been applied by reconcileOldMachineSetsRollingUpdate.
	for _, oldMS := range p.oldMSs {
		if ptr.Deref(oldMS.Status.AvailableReplicas, 0) == ptr.Deref(oldMS.Status.Replicas, 0) || ptr.Deref(oldMS.Spec.Replicas, 0) == 0 {
			continue
		}

		newScaleIntent := max(ptr.Deref(oldMS.Spec.Replicas, 0)-1, 0)
		p.addNotef(p.newMS, "scaling down by 1 to unblock rollout stuck due to unavailable Machine on oldMS")
		log.Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d) to unblock rollout stuck due to unavailable Machine on oldMS", klog.KObj(oldMS), newScaleIntent, 1), "MachineSet", klog.KObj(oldMS))
		p.scaleIntents[oldMS.Name] = newScaleIntent
		return
	}
}

func sortAndJoin(a []string) string {
	sort.Strings(a)
	return strings.Join(a, ",")
}

// requeueAfter returns after how long the MachineDeployment must be reconciled again to follow up on a rollout
// that is pending (waiting for an answer from an extension) or blocked (no allowed operation can roll out changes).
// It returns 0 if there is no need to requeue.
func (p *rolloutPlanner) requeueAfter() time.Duration {
	// Note: If at least one oldMS is pending, the entire rollout is paused and nothing can change before all the pending oldMSs
	// get a decision; blocked oldMSs are re-evaluated at that time, if their cached result expired in the meantime.
	retryAfter := p.pendingRetryAfter
	if retryAfter.IsZero() {
		retryAfter = p.blockedRetryAfter
	}
	if retryAfter.IsZero() {
		return 0
	}
	// Note: retryAfter can already be in the past (e.g. if a cached result expired in the meantime), a RequeueAfter <= 0 would not requeue at all.
	return max(retryAfter.Sub(p.now()), time.Second) // RequeueAfter should be at least 1s, because controller rate limiter is using 1s.
}
