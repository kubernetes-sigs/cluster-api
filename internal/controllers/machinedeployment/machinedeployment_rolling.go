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
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/patch"
)

// rolloutRolling implements the logic for rolling a new MachineSet.
func (r *Reconciler) rolloutRolling(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, templateExists bool) error {
	// TODO(in-place): move create newMS into rolloutPlanner
	newMS, oldMSs, err := r.getAllMachineSetsAndSyncRevision(ctx, md, msList, true, templateExists)
	if err != nil {
		return err
	}

	// newMS can be nil in case there is already a MachineSet associated with this deployment,
	// but there are only either changes in annotations or MinReadySeconds. Or in other words,
	// this can be nil if there are changes, but no replacement of existing machines is needed.
	if newMS == nil {
		return nil
	}

	allMSs := append(oldMSs, newMS)

	// TODO(in-place): also apply/remove labels to MS should go into rolloutPlanner
	if err := r.cleanupDisableMachineCreateAnnotation(ctx, newMS); err != nil {
		return err
	}

	planner := newRolloutPlanner()
	planner.md = md
	planner.newMS = newMS
	planner.oldMSs = oldMSs

	if err := planner.planRolloutRolling(ctx); err != nil {
		return err
	}

	// TODO(in-place): this should be changed as soon as rolloutPlanner support MS creation and adding/removing labels from MS
	for _, ms := range allMSs {
		scaleIntent := ptr.Deref(ms.Spec.Replicas, 0)
		if v, ok := planner.scaleIntents[ms.Name]; ok {
			scaleIntent = v
		}
		if err := r.scaleMachineSet(ctx, ms, scaleIntent, md); err != nil {
			return err
		}
	}

	if err := r.syncDeploymentStatus(allMSs, newMS, md); err != nil {
		return err
	}

	if mdutil.DeploymentComplete(md, &md.Status) {
		if err := r.cleanupDeployment(ctx, oldMSs, md); err != nil {
			return err
		}
	}

	return nil
}

type rolloutPlanner struct {
	md           *clusterv1.MachineDeployment
	newMS        *clusterv1.MachineSet
	oldMSs       []*clusterv1.MachineSet
	scaleIntents map[string]int32
}

func newRolloutPlanner() *rolloutPlanner {
	return &rolloutPlanner{
		scaleIntents: make(map[string]int32),
	}
}

// planRolloutRolling determine how to proceed with the rollout when using the RolloutRolling strategy if the system is not yet at the desired state.
func (p *rolloutPlanner) planRolloutRolling(ctx context.Context) error {
	if p.md.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineDeployment %v is nil, this is unexpected", client.ObjectKeyFromObject(p.md))
	}

	if p.newMS.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(p.newMS))
	}

	for _, oldMS := range p.oldMSs {
		if oldMS.Spec.Replicas == nil {
			return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(oldMS))
		}
	}

	// Scale up, if we can.
	if err := p.reconcileNewMachineSet(ctx); err != nil {
		return err
	}

	// Scale down, if we can.
	if err := p.reconcileOldMachineSetsRolloutRolling(ctx); err != nil {
		return err
	}

	// This func tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
	// Note: This func must be called after computing scale up/down intent for all the MachineSets.
	// Note: This func only addresses deadlocks due to unavailable replicas not getting deleted on oldMSs, which can happen
	// because reconcileOldMachineSetsRolloutRolling called above always assumes the worst case when deleting replicas e.g.
	//  - MD with spec.replicas 3, MaxSurge 1, MaxUnavailable 0
	//  - OldMS with 3 replicas, 2 available replica (and thus 1 unavailable replica)
	//  - NewMS with 1 replica, 1 available replica
	//  - In theory it is possible to scale down oldMS from 3->2 replicas by deleting the unavailable replica.
	//  - However, reconcileOldMachineSetsRolloutRolling cannot assume that the MachineSet controller is going to delete
	//    the unavailable replica when scaling down from 3->2, because it might happen that one of the available replicas
	//    is deleted instead.
	//  - As a consequence reconcileOldMachineSetsRolloutRolling, which assumes the worst case when deleting replicas, did not scaled down oldMS.
	// This situation, rollout not proceeding due to unavailable replicas, is considered a deadlock to be addressed by reconcileDeadlockBreaker.
	// Note: Unblocking deadlocks when unavailable replicas exist only on oldMSs, is required also because replicas on oldMSs are not remediated by MHC.
	p.reconcileDeadlockBreaker(ctx)
	return nil
}

func (p *rolloutPlanner) reconcileNewMachineSet(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(p.oldMSs, p.newMS)

	if *(p.newMS.Spec.Replicas) == *(p.md.Spec.Replicas) {
		// Scaling not required.
		return nil
	}

	if *(p.newMS.Spec.Replicas) > *(p.md.Spec.Replicas) {
		// Scale down.
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas", p.newMS.Name, *(p.md.Spec.Replicas)), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = *(p.md.Spec.Replicas)
		return nil
	}

	newReplicasCount, err := mdutil.NewMSNewReplicas(p.md, allMSs, *p.newMS.Spec.Replicas)
	if err != nil {
		return err
	}

	if newReplicasCount < *(p.newMS.Spec.Replicas) {
		scaleDownCount := *(p.newMS.Spec.Replicas) - newReplicasCount
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", p.newMS.Name, newReplicasCount, scaleDownCount), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = newReplicasCount
	}
	if newReplicasCount > *(p.newMS.Spec.Replicas) {
		scaleUpCount := newReplicasCount - *(p.newMS.Spec.Replicas)
		log.V(5).Info(fmt.Sprintf("Setting scale up intent for MachineSet %s to %d replicas (+%d)", p.newMS.Name, newReplicasCount, scaleUpCount), "MachineSet", klog.KObj(p.newMS))
		p.scaleIntents[p.newMS.Name] = newReplicasCount
	}
	return nil
}

func (p *rolloutPlanner) reconcileOldMachineSetsRolloutRolling(ctx context.Context) error {
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

	// Compute the total number of replicas that can be scaled down.
	// Exit immediately if there is no room for scaling down.
	// NOTE: this is a quick preliminary check to verify if there is room for scaling down any of the oldMSs; further down the code
	// will make additional checks to ensure scale down actually happens without breaching MaxUnavailable, and
	// if necessary, it will reduce the extent of the scale down accordingly.
	totalScaleDownCount := max(totalSpecReplicas-totalPendingScaleDown-minAvailable, 0)
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
			replicas = min(replicas, v)
		}

		// No op if this MS has been already scaled down to zero.
		if replicas <= 0 {
			continue
		}

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
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", oldMS.Name, newScaleIntent, scaleDown), "MachineSet", klog.KObj(oldMS))
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
	// Note: we are counting only pending scale up & down from the current status.replicas (so actual scale up & down of replicas number, not any other possible "re-alignment" of spec.replicas).
	for _, ms := range allMSs {
		scaleIntent := ptr.Deref(ms.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[ms.Name]; ok {
			scaleIntent = v
		}
		if scaleIntent != ptr.Deref(ms.Status.Replicas, 0) {
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
	// Note: in most cases this is only a formal violation of maxUnavailability, because there is a good chance
	// that the machine that will be deleted is one of the unavailable machines.
	for _, oldMS := range p.oldMSs {
		if ptr.Deref(oldMS.Status.AvailableReplicas, 0) == ptr.Deref(oldMS.Status.Replicas, 0) || ptr.Deref(oldMS.Spec.Replicas, 0) == 0 {
			continue
		}

		newScaleIntent := max(ptr.Deref(oldMS.Spec.Replicas, 0)-1, 0)
		log.Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d) to unblock rollout stuck due to unavailable Machine on oldMS", oldMS.Name, newScaleIntent, 1), "MachineSet", klog.KObj(oldMS))
		p.scaleIntents[oldMS.Name] = newScaleIntent
		return
	}
}

// cleanupDisableMachineCreateAnnotation will remove the disable machine create annotation from new MachineSets that were created during reconcileOldMachineSetsOnDelete.
func (r *Reconciler) cleanupDisableMachineCreateAnnotation(ctx context.Context, newMS *clusterv1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx, "MachineSet", klog.KObj(newMS))

	if newMS.Annotations != nil {
		if _, ok := newMS.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok {
			log.V(4).Info("removing annotation on latest MachineSet to enable machine creation")
			patchHelper, err := patch.NewHelper(newMS, r.Client)
			if err != nil {
				return err
			}
			delete(newMS.Annotations, clusterv1.DisableMachineCreateAnnotation)
			err = patchHelper.Patch(ctx, newMS)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
