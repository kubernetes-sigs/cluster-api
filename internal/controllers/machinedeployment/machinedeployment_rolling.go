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
	if err := planner.Plan(ctx, md, newMS, oldMSs); err != nil {
		return err
	}

	// TODO(in-place): this should be changed as soon as rolloutPlanner support MS creation and adding/removing labels from MS
	for _, ms := range allMSs {
		if scaleIntent, ok := planner.scaleIntents[ms.Name]; ok {
			if err := r.scaleMachineSet(ctx, ms, scaleIntent, md); err != nil {
				return err
			}
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
	scaleIntents map[string]int32
}

func newRolloutPlanner() *rolloutPlanner {
	return &rolloutPlanner{
		scaleIntents: make(map[string]int32),
	}
}

// Plan determine if it is.
func (p *rolloutPlanner) Plan(ctx context.Context, md *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) error {
	if md.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineDeployment %v is nil, this is unexpected", client.ObjectKeyFromObject(md))
	}

	if newMS.Spec.Replicas == nil {
		return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(newMS))
	}

	for _, oldMS := range oldMSs {
		if oldMS.Spec.Replicas == nil {
			return errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(oldMS))
		}
	}

	// Scale up, if we can.
	if err := p.reconcileNewMachineSet(ctx, md, newMS, oldMSs); err != nil {
		return err
	}

	// Scale down, if we can.
	return p.reconcileOldMachineSets(ctx, md, newMS, oldMSs)
}

func (p *rolloutPlanner) reconcileNewMachineSet(ctx context.Context, md *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(oldMSs, newMS)

	if *(newMS.Spec.Replicas) == *(md.Spec.Replicas) {
		// Scaling not required.
		return nil
	}

	if *(newMS.Spec.Replicas) > *(md.Spec.Replicas) {
		// Scale down.
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas", newMS.Name, *(md.Spec.Replicas)), "machineset", client.ObjectKeyFromObject(newMS).String())
		p.scaleIntents[newMS.Name] = *(md.Spec.Replicas)
		return nil
	}

	newReplicasCount, err := mdutil.NewMSNewReplicas(md, allMSs, *newMS.Spec.Replicas)
	if err != nil {
		return err
	}

	if newReplicasCount < *(newMS.Spec.Replicas) {
		scaleDownCount := *(newMS.Spec.Replicas) - newReplicasCount
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas (-%d)", newMS.Name, newReplicasCount, scaleDownCount), "machineset", client.ObjectKeyFromObject(newMS).String())
		p.scaleIntents[newMS.Name] = newReplicasCount
	}
	if newReplicasCount > *(newMS.Spec.Replicas) {
		scaleUpCount := newReplicasCount - *(newMS.Spec.Replicas)
		log.V(5).Info(fmt.Sprintf("Setting scale up intent for %s to %d replicas (+%d)", newMS.Name, newReplicasCount, scaleUpCount), "machineset", client.ObjectKeyFromObject(newMS).String())
		p.scaleIntents[newMS.Name] = newReplicasCount
	}
	return nil
}

func (p *rolloutPlanner) reconcileOldMachineSets(ctx context.Context, md *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) error {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(oldMSs, newMS)

	oldMachinesCount := mdutil.GetReplicaCountForMachineSets(oldMSs)
	if oldMachinesCount == 0 {
		// Can't scale down further
		return nil
	}

	allMachinesCount := mdutil.GetReplicaCountForMachineSets(allMSs)
	log.V(4).Info("New MachineSet has available machines",
		"machineset", client.ObjectKeyFromObject(newMS).String(), "available-replicas", ptr.Deref(newMS.Status.AvailableReplicas, 0))
	maxUnavailable := mdutil.MaxUnavailable(*md)

	// Check if we can scale down. We can scale down in the following 2 cases:
	// * Some old MachineSets have unhealthy replicas, we could safely scale down those unhealthy replicas since that won't further
	//  increase unavailability.
	// * New MachineSet has scaled up and it's replicas becomes ready, then we can scale down old MachineSets in a further step.
	//
	// maxScaledDown := allMachinesCount - minAvailable - newMachineSetMachinesUnavailable
	// take into account not only maxUnavailable and any surge machines that have been created, but also unavailable machines from
	// the newMS, so that the unavailable machines from the newMS would not make us scale down old MachineSets in a further
	// step(that will increase unavailability).
	//
	// Concrete example:
	//
	// * 10 replicas
	// * 2 maxUnavailable (absolute number, not percent)
	// * 3 maxSurge (absolute number, not percent)
	//
	// case 1:
	// * Deployment is updated, newMS is created with 3 replicas, oldMS is scaled down to 8, and newMS is scaled up to 5.
	// * The new MachineSet machines crashloop and never become available.
	// * allMachinesCount is 13. minAvailable is 8. newMSMachinesUnavailable is 5.
	// * A node fails and causes one of the oldMS machines to become unavailable. However, 13 - 8 - 5 = 0, so the oldMS won't be scaled down.
	// * The user notices the crashloop and does kubectl rollout undo to rollback.
	// * newMSMachinesUnavailable is 1, since we rolled back to the good MachineSet, so maxScaledDown = 13 - 8 - 1 = 4. 4 of the crashlooping machines will be scaled down.
	// * The total number of machines will then be 9 and the newMS can be scaled up to 10.
	//
	// case 2:
	// Same example, but pushing a new machine template instead of rolling back (aka "roll over"):
	// * The new MachineSet created must start with 0 replicas because allMachinesCount is already at 13.
	// * However, newMSMachinesUnavailable would also be 0, so the 2 old MachineSets could be scaled down by 5 (13 - 8 - 0), which would then
	// allow the new MachineSet to be scaled up by 5.
	availableReplicas := ptr.Deref(newMS.Status.AvailableReplicas, 0)

	minAvailable := *(md.Spec.Replicas) - maxUnavailable
	newMSUnavailableMachineCount := *(newMS.Spec.Replicas) - availableReplicas
	maxScaledDown := allMachinesCount - minAvailable - newMSUnavailableMachineCount
	if maxScaledDown <= 0 {
		return nil
	}

	// Clean up unhealthy replicas first, otherwise unhealthy replicas will block deployment
	// and cause timeout. See https://github.com/kubernetes/kubernetes/issues/16737
	oldMSs, cleanupCount, err := p.cleanupUnhealthyReplicas(ctx, oldMSs, maxScaledDown)
	if err != nil {
		return err
	}

	log.V(4).Info("Cleaned up unhealthy replicas from old MachineSets", "count", cleanupCount)

	// Scale down old MachineSets, need check maxUnavailable to ensure we can scale down
	scaledDownCount, err := p.scaleDownOldMachineSetsForRollingUpdate(ctx, md, newMS, oldMSs)
	if err != nil {
		return err
	}

	log.V(4).Info("Scaled down old MachineSets of MachineDeployment", "count", scaledDownCount)
	return nil
}

// cleanupUnhealthyReplicas will scale down old MachineSets with unhealthy replicas, so that all unhealthy replicas will be deleted.
func (p *rolloutPlanner) cleanupUnhealthyReplicas(ctx context.Context, oldMSs []*clusterv1.MachineSet, maxCleanupCount int32) ([]*clusterv1.MachineSet, int32, error) {
	log := ctrl.LoggerFrom(ctx)

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(oldMSs))

	// Scale down all old MachineSets with any unhealthy replicas. MachineSet will honour spec.deletion.order
	// for deleting Machines. Machines with a deletion timestamp, with a failure message or without a nodeRef
	// are preferred for all strategies.
	// This results in a best effort to remove machines backing unhealthy nodes.
	totalScaledDown := int32(0)

	for _, oldMS := range oldMSs {
		if oldMS.Spec.Replicas == nil {
			return nil, 0, errors.Errorf("spec.replicas for MachineSet %v is nil, this is unexpected", client.ObjectKeyFromObject(oldMS))
		}

		if totalScaledDown >= maxCleanupCount {
			break
		}

		oldMSReplicas := *(oldMS.Spec.Replicas)
		if oldMSReplicas == 0 {
			// cannot scale down this MachineSet.
			continue
		}

		oldMSAvailableReplicas := ptr.Deref(oldMS.Status.AvailableReplicas, 0)
		log.V(4).Info("Found available Machines in old MachineSet",
			"count", oldMSAvailableReplicas, "target-machineset", client.ObjectKeyFromObject(oldMS).String())
		if oldMSReplicas == oldMSAvailableReplicas {
			// no unhealthy replicas found, no scaling required.
			continue
		}

		// TODO(in-place): fix this logic
		//  It looks like that the current logic fails when the MD controller is called twice in a row, without MS controller being triggered in the between, e.g.
		//  - first reconcile scales down ms1, 6-->5 (-1)
		//  - second reconcile is not taking into account scales down already in progress, unhealthy count is wrongly computed as -1 instead of 0, this leads to increasing replica count instead of keeping it as it is (or scaling down), and then the safeguard below errors out.

		remainingCleanupCount := maxCleanupCount - totalScaledDown
		unhealthyCount := oldMSReplicas - oldMSAvailableReplicas
		scaledDownCount := min(remainingCleanupCount, unhealthyCount)
		newReplicasCount := oldMSReplicas - scaledDownCount

		if newReplicasCount > oldMSReplicas {
			return nil, 0, errors.Errorf("when cleaning up unhealthy replicas, got invalid request to scale down %v: %d -> %d",
				client.ObjectKeyFromObject(oldMS), oldMSReplicas, newReplicasCount)
		}

		scaleDownCount := *(oldMS.Spec.Replicas) - newReplicasCount
		log.V(5).Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas (-%d)", oldMS.Name, newReplicasCount, scaleDownCount), "machineset", client.ObjectKeyFromObject(oldMS).String())
		p.scaleIntents[oldMS.Name] = newReplicasCount

		totalScaledDown += scaledDownCount
	}

	return oldMSs, totalScaledDown, nil
}

// scaleDownOldMachineSetsForRollingUpdate scales down old MachineSets when deployment strategy is "RollingUpdate".
// Need check maxUnavailable to ensure availability.
func (p *rolloutPlanner) scaleDownOldMachineSetsForRollingUpdate(ctx context.Context, md *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) (int32, error) {
	log := ctrl.LoggerFrom(ctx)
	allMSs := append(oldMSs, newMS)

	maxUnavailable := mdutil.MaxUnavailable(*md)
	minAvailable := *(md.Spec.Replicas) - maxUnavailable

	// Find the number of available machines.
	availableMachineCount := ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(allMSs), 0)

	// Check if we can scale down.
	if availableMachineCount <= minAvailable {
		// Cannot scale down.
		return 0, nil
	}

	log.V(4).Info("Found available machines in deployment, scaling down old MSes", "count", availableMachineCount)

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(oldMSs))

	// TODO(in-place): fix this logic
	//  It looks like that the current logic fails when the MD controller is called twice in a row e.g. reconcile of md 6 replicas MaxSurge=3, MaxUnavailable=1 and
	//     ms1, 6/5 replicas << one is scaling down, but scale down not yet processed by the MS controller.
	//     ms2, 3/3 replicas
	//  Leads to:
	//     ms1, 6/1 replicas << it further scaled down by 4, which leads to totAvailable machines is less than MinUnavailable, which should not happen
	//     ms2, 3/3 replicas

	totalScaledDown := int32(0)
	totalScaleDownCount := availableMachineCount - minAvailable
	for _, oldMS := range oldMSs {
		if totalScaledDown >= totalScaleDownCount {
			// No further scaling required.
			break
		}

		scaleIntent := ptr.Deref(oldMS.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[oldMS.Name]; ok {
			scaleIntent = v
		}

		if scaleIntent == 0 {
			// cannot scale down this MachineSet.
			continue
		}

		// Scale down.
		scaleDownCount := min(scaleIntent, totalScaleDownCount-totalScaledDown)
		newReplicasCount := scaleIntent - scaleDownCount
		if newReplicasCount > scaleIntent {
			return totalScaledDown, errors.Errorf("when scaling down old MachineSet, got invalid request to scale down %v: %d -> %d",
				client.ObjectKeyFromObject(oldMS), scaleIntent, newReplicasCount)
		}

		log.V(5).Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas (-%d)", oldMS.Name, newReplicasCount, scaleDownCount), "machineset", client.ObjectKeyFromObject(oldMS).String())
		p.scaleIntents[oldMS.Name] = newReplicasCount

		totalScaledDown += scaleDownCount
	}

	return totalScaledDown, nil
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
