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

	// Perform the can update MachineSet decision.
	if err := p.canUpdateMachineSetInPlaceDecisions(ctx); err != nil {
		return err
	}

	// Scale up, if we can.
	if err := p.reconcileNewMachineSet(ctx); err != nil {
		return err
	}

	// Scale down, if we can.
	if err := p.reconcileOldMachineSetsRollingUpdate(ctx); err != nil {
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

// canUpdateMachineSetInPlaceDecisions call the extensions registered for the canUdateMachineSet and
// perform the decision about how to rollout changes for each one of the OldMs.
// Note: If the InPlaceUpdates feature gate is off, this method is a no-op.
func (p *rolloutPlanner) canUpdateMachineSetInPlaceDecisions(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if !feature.Gates.Enabled(feature.InPlaceUpdates) {
		return nil
	}

	// If new MS already has all desired replicas, it does not make sense to perform more in-place updates.
	if ptr.Deref(p.newMS.Spec.Replicas, 0) >= ptr.Deref(p.md.Spec.Replicas, 0) {
		return nil
	}

	// Determine the rollout decision for each oldMS.
	p.pendingMSs = sets.Set[string]{}
	p.blockedMSs = sets.Set[string]{}
	p.inPlaceUpdateCandidates = sets.Set[string]{}
	p.inPlaceUpdateCandidatesNotAffectingAvailability = sets.Set[string]{}
	p.replaceCandidates = sets.Set[string]{}
	for _, oldMS := range p.oldMSs {
		// If the oldMS doesn't have replicas anymore, nothing left to do.
		if ptr.Deref(oldMS.Status.Replicas, 0) <= 0 {
			continue
		}

		// If the oldMS is not eligible for in-place updates, changes can be rolled out only by replacing machines.
		// Note: The extension is not called in this case, so it cannot block the rollout.
		if result, ok := p.upToDateResults[oldMS.Name]; !ok || !result.EligibleForInPlaceUpdate {
			p.replaceCandidates.Insert(oldMS.Name)
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
			p.pendingMSs.Insert(oldMS.Name)
			// Note: The rollout stays paused until all the pending oldMSs get a decision, and cached pending results are used until
			// their retryAfter, so the pause cannot be lifted before the latest retryAfter.
			if res.retryAfter.After(p.pendingRetryAfter) {
				p.pendingRetryAfter = res.retryAfter
			}
			p.canUpdateMachineSetResults.waitingForDecisionMessages = append(p.canUpdateMachineSetResults.waitingForDecisionMessages, canUpdateMachineSetHookMessage(oldMS, strings.Join(res.messages, ", ")))
			p.addNotef(oldMS, "rollout paused, waiting for the extension to take a decision about how to roll out changes (retry after %s)", res.retryAfter.Sub(p.now()).Truncate(time.Second))
		case oldMSRolloutBlockedDecision:
			p.blockedMSs.Insert(oldMS.Name)
			// Note: Blocked oldMSs are re-evaluated independently, so we have to reconcile when the first cached result expires.
			if p.blockedRetryAfter.IsZero() || res.retryAfter.Before(p.blockedRetryAfter) {
				p.blockedRetryAfter = res.retryAfter
			}
			msg := blockedMessage(res)
			p.canUpdateMachineSetResults.rolloutBlockedMessages = append(p.canUpdateMachineSetResults.rolloutBlockedMessages, canUpdateMachineSetHookMessage(oldMS, msg))
			p.addNotef(oldMS, "rollout blocked, %s", msg)
			log.V(5).Info(fmt.Sprintf("MachineSet %s cannot be rolled out: rollout is blocked by CanUpdateMachineSet hook: %s", klog.KObj(oldMS), msg), "MachineSet", klog.KObj(oldMS))
		case oldMSRolloutReplaceDecision:
			p.replaceCandidates.Insert(oldMS.Name)
			if res.canUpdateMachineSet {
				p.addNotef(oldMS, "in-place update is possible but not allowed, falling back to replace machines")
			}
		case oldMSRolloutUpdateInPlaceDecision:
			p.inPlaceUpdateCandidates.Insert(oldMS.Name)
			if res.affectsAvailability {
				p.addNotef(oldMS, "should scale down by moving Machines to MachineSet %s (affects availability)", p.newMS.Name)
			} else {
				p.inPlaceUpdateCandidatesNotAffectingAvailability.Insert(oldMS.Name)
				p.addNotef(oldMS, "should scale down by moving Machines to MachineSet %s (does not affect availability)", p.newMS.Name)
			}
			// Note: The annotations triggering the move are set by reconcileOldMachineSetsRollingUpdate, only if the rollout is not paused.
		}
	}
	return nil
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

	// Note: If the InPlaceUpdates feature gate is off, maxSurgeLimit will not be set and NewMSNewReplicas will
	// use maxSurge as usual when necessary.
	// Note: maxSurge is also used at its fullest extent when there are only replaceCandidates (no pendingMS, no inPlaceUpdateCandidates).
	// Note: scale up/down due to changes to MachineDeployment spec.replicas still happen.
	var maxSurgeLimit *int
	switch {
	case p.pendingMSs.Len() > 0:
		maxSurgeLimit = new(0)
		p.addNotef(p.newMS, "Rollout paused for MachineSet %s, maxSurge cannot be used for %s", sortAndJoin(p.pendingMSs.UnsortedList()), p.newMS.Name)
	case p.replaceCandidates.Len() == 0 && p.inPlaceUpdateCandidates.Len() == 0 && p.blockedMSs.Len() > 0: // everything is blocked
		maxSurgeLimit = new(0)
		p.addNotef(p.newMS, "Rollout blocked for MachineSet %s, maxSurge cannot be used for %s", sortAndJoin(p.blockedMSs.UnsortedList()), p.newMS.Name)
	case p.inPlaceUpdateCandidatesNotAffectingAvailability.Len() > 0:
		maxSurgeLimit = new(0)
		p.addNotef(p.newMS, "MachineSet %s can rollout in place without affecting availability, maxSurge is not required for %s", sortAndJoin(p.inPlaceUpdateCandidatesNotAffectingAvailability.UnsortedList()), p.newMS.Name)
	case p.inPlaceUpdateCandidates.Len() > 0 && mdutil.MaxUnavailable(*p.md) > 0:
		maxSurgeLimit = new(0)
		p.addNotef(p.newMS, "MachineSet %s can rollout in place affecting availability, maxUnavailable is greater than zero, maxSurge is not required for %s", sortAndJoin(p.inPlaceUpdateCandidates.UnsortedList()), p.newMS.Name)
	case p.inPlaceUpdateCandidates.Len() > 0 && mdutil.MaxUnavailable(*p.md) == 0:
		p.addNotef(p.newMS, "MachineSet %s can rollout in place affecting availability, maxSurge limit set to 1 for %s", sortAndJoin(p.inPlaceUpdateCandidates.UnsortedList()), p.newMS.Name)
		maxSurgeLimit = new(1)
	}

	newReplicasCount, note, err := mdutil.NewMSNewReplicas(p.md, allMSs, *p.newMS.Spec.Replicas, maxSurgeLimit)
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

// reconcileOldMachineSetsRollingUpdate computes the scale down intents for the oldMSs, oldest first, without breaching
// the MaxUnavailable limits of the MachineDeployment.
func (p *rolloutPlanner) reconcileOldMachineSetsRollingUpdate(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(p.oldMSs, p.newMS)

	// If at least one oldMS is waiting for a decision from the extension, pause the rollout.
	// Note: If the InPlaceUpdates feature gate is off, pendingMSs will always be empty.
	if p.pendingMSs.Len() > 0 {
		// Note: scale down due to changes to MachineDeployment spec.replicas still happen.
		p.scaleDownExcessReplicas(ctx)
		return nil
	}

	// Set annotations so oldMSs for which the decision was UpdateInPlace are informed that they have to move machines to the newMS;
	// Also set the annotation on the newMS so it is ready to accept those replicas.
	// Note: This must happen before the check below, because an oldMS can have spec.replicas 0 and still have Machines to be moved,
	// and the annotation must stay in place until the move is completed.
	// Note: If the InPlaceUpdates feature gate is off, this method is a no-op.
	if err := p.setMoveMachinesAnnotations(); err != nil {
		return err
	}

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
		// If there is room for scaling down, but there are inPlaceUpdateCandidatesNotAffectingAvailability, and no scale down is in progress,
		// scale down by one.
		// Note: If the InPlaceUpdates feature gate is off, inPlaceUpdateCandidatesNotAffectingAvailability will always be empty.
		if p.inPlaceUpdateCandidatesNotAffectingAvailability.Len() > 0 && totalInPlaceUpdatingNotAffectingAvailability == 0 && totalPendingScaleDown == 0 {
			// Scale down one of the oldMS candidate for in-place update and that can perform an update without affecting availability.
			// Sort oldMSs so the system will start moving machines from the oldest MS first.
			candidates := make([]*clusterv1.MachineSet, 0, p.inPlaceUpdateCandidatesNotAffectingAvailability.Len())
			for _, oldMS := range p.oldMSs {
				if p.inPlaceUpdateCandidatesNotAffectingAvailability.Has(oldMS.Name) {
					candidates = append(candidates, oldMS)
				}
			}
			sort.Sort(mdutil.MachineSetsByCreationTimestamp(candidates))
			oldMS := candidates[0]

			newScaleDownIntentOldMS := max(p.replicasAfterScaleIntent(oldMS)-1, 0)
			p.addNotef(oldMS, "scale down for MachineSet %s to %d replicas (-%d) allowed for in place update not affecting availability", oldMS.Name, newScaleDownIntentOldMS, 1)
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", klog.KObj(oldMS), newScaleDownIntentOldMS, 1), "MachineSet", klog.KObj(oldMS))
			p.scaleIntents[oldMS.Name] = newScaleDownIntentOldMS
		}
		return nil
	}

	// Do not scale down the oldMSs for which the canUpdateMachineSet decision is to block the rollout.
	scaleDownCandidates := make([]*clusterv1.MachineSet, 0, len(p.oldMSs))
	for _, oldMS := range p.oldMSs {
		if !p.blockedMSs.Has(oldMS.Name) {
			scaleDownCandidates = append(scaleDownCandidates, oldMS)
		}
	}

	// Sort candidates so the system will start deleting from the oldest MS first.
	sort.Sort(mdutil.MachineSetsByCreationTimestamp(scaleDownCandidates))

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
	totalScaleDownCount, totalAvailableReplicas = p.scaleDownOldMSs(ctx, scaleDownCandidates, totalScaleDownCount, totalAvailableReplicas, minAvailable, true)

	// Then scale down old MS down to zero replicas / down to residual totalScaleDownCount.
	// NOTE: Also in this case, we should continuously assess if reducing the number of replicase could further impact availability,
	// and if necessary, limit scale down extent to ensure the operation respects MaxUnavailable limits.
	_, _ = p.scaleDownOldMSs(ctx, scaleDownCandidates, totalScaleDownCount, totalAvailableReplicas, minAvailable, false)

	return nil
}

// setMoveMachinesAnnotations informing the oldMS that it must move machines to the newMS, and the newMS to accept those replicas.
// Note: Cleanup of the move annotations will happen automatically as soon as the rollout planner stops
// to set them, because move annotations are not part of the output of computeDesiredMS.
// Note: If the InPlaceUpdates feature gate is off, this method is a no-op.
func (p *rolloutPlanner) setMoveMachinesAnnotations() error {
	for _, oldMS := range p.oldMSs {
		if !p.inPlaceUpdateCandidates.Has(oldMS.Name) {
			continue
		}

		// Set the annotation informing the oldMS that it must move machines to the newMS instead of deleting them.
		// Note: After a machine is moved from oldMS to newMS, the newMS will take care of the in-place update process.
		affectsAvailability := !p.inPlaceUpdateCandidatesNotAffectingAvailability.Has(oldMS.Name)
		if oldMS.Annotations == nil {
			oldMS.Annotations = map[string]string{}
		}
		moveMachinesToMachineSetAnnotationData := clusterv1.MachineSetMoveMachinesToMachineSetAnnotationData{
			Name:                p.newMS.Name,
			AffectsAvailability: new(affectsAvailability),
		}
		dataBytes, err := json.Marshal(moveMachinesToMachineSetAnnotationData)
		if err != nil {
			return pkgerrors.Wrapf(err, "failed to trigger in-place update for MachineSet %s by setting the %s annotation: failed to Marshal data", klog.KObj(oldMS), clusterv1.MachineSetMoveMachinesToMachineSetAnnotation)
		}
		oldMS.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation] = string(dataBytes)
	}

	// Set the annotation informing the newMS that it will receive replicas moved from oldMS selected as in-place candidates.
	// Note: there is a two-ways check before the move operation:
	if p.inPlaceUpdateCandidates.Len() > 0 {
		if p.newMS.Annotations == nil {
			p.newMS.Annotations = map[string]string{}
		}
		p.newMS.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation] = sortAndJoin(p.inPlaceUpdateCandidates.UnsortedList())
	}
	return nil
}

// scaleDownExcessReplicas sets the scale down intent for the oldMSs, oldest first, when the MachineSets have more replicas
// than MachineDeployment spec.replicas.
// Note: This is a plain scale down, so Machines are deleted: it does not depend on the CanUpdateMachineSet decisions and,
// like the scale down of the newMS in reconcileNewMachineSet, it does not consider minAvailable because it is requested by the user.
func (p *rolloutPlanner) scaleDownExcessReplicas(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)

	mdReplicas := ptr.Deref(p.md.Spec.Replicas, 0)
	maxSurge := mdutil.MaxSurge(*p.md)

	// Note: Replicas are computed after applying the intents already set in this reconcile, e.g. newMS scale down.
	totalReplicas := p.replicasAfterScaleIntent(p.newMS)
	for _, oldMS := range p.oldMSs {
		totalReplicas += p.replicasAfterScaleIntent(oldMS)
	}

	excessReplicas := totalReplicas - (mdReplicas + maxSurge)
	if excessReplicas <= 0 {
		return
	}

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(p.oldMSs))
	for _, oldMS := range p.oldMSs {
		if excessReplicas <= 0 {
			break
		}
		replicas := p.replicasAfterScaleIntent(oldMS)
		scaleDown := min(replicas, excessReplicas)
		if scaleDown <= 0 {
			continue
		}
		p.scaleIntents[oldMS.Name] = replicas - scaleDown
		excessReplicas -= scaleDown
		p.addNotef(oldMS, "%d current Machines > %d MachineDeployment spec.replicas + %d maxSurge", totalReplicas, mdReplicas, maxSurge)
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", klog.KObj(oldMS), replicas-scaleDown, scaleDown), "MachineSet", klog.KObj(oldMS))
	}
}

// scaleDownOldMSs sets the scale down intent for the given oldMSs, in order, up to totalScaleDownCount replicas.
func (p *rolloutPlanner) scaleDownOldMSs(ctx context.Context, oldMSs []*clusterv1.MachineSet, totalScaleDownCount, totalAvailableReplicas, minAvailable int32, scaleDownOnlyUnavailableReplicas bool) (int32, int32) {
	log := ctrl.LoggerFrom(ctx)

	for _, oldMS := range oldMSs {
		// No op if there is no scaling down left.
		if totalScaleDownCount <= 0 {
			break
		}

		// No op if this MS has been already scaled down to zero.
		replicas := p.replicasAfterScaleIntent(oldMS)
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

// This funcs tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
// Note: This func must be called after computing scale up/down intent for all the MachineSets.
// Note: This func only address deadlock due to unavailable machines not getting deleted on oldMSs, e.g. due to a wrong configuration.
// Note: Unblocking deadlocks when unavailable replicas exist only on oldMSs, is required also because replicas on oldMSs are not remediated by MHC.
func (p *rolloutPlanner) reconcileDeadlockBreaker(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(p.oldMSs, p.newMS)

	// If the rollout is paused waiting for a decision from the extension, or blocked because no allowed operation can roll out changes,
	// this is not a deadlock, it is the expected state.
	if p.pendingMSs.Len() > 0 || p.blockedMSs.Len() > 0 {
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
	// RequeueAfter should be at least 1s, because controller rate limiter is using 1s.
	return max(retryAfter.Sub(p.now()), time.Second)
}

// rolloutDecision returns how to roll out changes for the oldMS, considering that allowedOperations is orthogonal to
// the fact that the extension can cover the entire diff with patches.
func rolloutDecision(r canUpdateMachineSetResult) oldMSRolloutDecision {
	switch {
	case r.retryAfterFromExtension:
		return oldMSRolloutPendingDecision
	case r.canUpdateMachineSet && isOperationAllowed(r.allowedOperations, runtimehooksv1.MachineUpdateOperationUpdateInPlace):
		return oldMSRolloutUpdateInPlaceDecision
	case isOperationAllowed(r.allowedOperations, runtimehooksv1.MachineUpdateOperationReplace):
		// Note: The oldMSRolloutReplaceDecision decision happens if
		// - In place update is not possible for the specific change
		// - In place update is possible, but not allowed
		return oldMSRolloutReplaceDecision
	default:
		return oldMSRolloutBlockedDecision
	}
}

func canUpdateMachineSetHookMessage(oldMS *clusterv1.MachineSet, details string) string {
	msg := fmt.Sprintf("Blocked by CanUpdateMachineSet hook for MachineSet %s", oldMS.Name)
	if details != "" {
		msg = fmt.Sprintf("%s: %s", msg, details)
	}
	return msg
}

func blockedMessage(res canUpdateMachineSetResult) string {
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
	return msg
}

func sortAndJoin(a []string) string {
	sort.Strings(a)
	return strings.Join(a, ",")
}
