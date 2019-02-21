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
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/integer"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	dutil "sigs.k8s.io/cluster-api/pkg/controller/machinedeployment/util"
)

// rolloutRolling implements the logic for rolling a new machine set.
func (r *ReconcileMachineDeployment) rolloutRolling(d *v1alpha1.MachineDeployment, msList []*v1alpha1.MachineSet, machineMap map[types.UID]*v1alpha1.MachineList) error {
	newMS, oldMSs, err := r.getAllMachineSetsAndSyncRevision(d, msList, machineMap, true)
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

	// Scale up, if we can.
	if err := r.reconcileNewMachineSet(allMSs, newMS, d); err != nil {
		return err
	}

	if err := r.syncDeploymentStatus(allMSs, newMS, d); err != nil {
		return err
	}

	// Scale down, if we can.
	if err := r.reconcileOldMachineSets(allMSs, oldMSs, newMS, d); err != nil {
		return err
	}

	if err := r.syncDeploymentStatus(allMSs, newMS, d); err != nil {
		return err
	}

	if dutil.DeploymentComplete(d, &d.Status) {
		if err := r.cleanupDeployment(oldMSs, d); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileMachineDeployment) reconcileNewMachineSet(allMSs []*v1alpha1.MachineSet, newMS *v1alpha1.MachineSet, deployment *v1alpha1.MachineDeployment) error {
	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for deployment set %v is nil, this is unexpected", deployment.Name)
	}

	if newMS.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", newMS.Name)
	}

	if *(newMS.Spec.Replicas) == *(deployment.Spec.Replicas) {
		// Scaling not required.
		return nil
	}

	if *(newMS.Spec.Replicas) > *(deployment.Spec.Replicas) {
		// Scale down.
		_, err := r.scaleMachineSet(newMS, *(deployment.Spec.Replicas), deployment)
		return err
	}

	newReplicasCount, err := dutil.NewMSNewReplicas(deployment, allMSs, newMS)
	if err != nil {
		return err
	}
	_, err = r.scaleMachineSet(newMS, newReplicasCount, deployment)
	return err
}

func (r *ReconcileMachineDeployment) reconcileOldMachineSets(allMSs []*v1alpha1.MachineSet, oldMSs []*v1alpha1.MachineSet, newMS *v1alpha1.MachineSet, deployment *v1alpha1.MachineDeployment) error {
	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for deployment set %v is nil, this is unexpected", deployment.Name)
	}

	if newMS.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", newMS.Name)
	}

	oldMachinesCount := dutil.GetReplicaCountForMachineSets(oldMSs)
	if oldMachinesCount == 0 {
		// Can't scale down further
		return nil
	}

	allMachinesCount := dutil.GetReplicaCountForMachineSets(allMSs)
	klog.V(4).Infof("New machine set %s/%s has %d available machines.", newMS.Namespace, newMS.Name, newMS.Status.AvailableReplicas)
	maxUnavailable := dutil.MaxUnavailable(*deployment)

	// Check if we can scale down. We can scale down in the following 2 cases:
	// * Some old machine sets have unhealthy replicas, we could safely scale down those unhealthy replicas since that won't further
	//  increase unavailability.
	// * New machine set has scaled up and it's replicas becomes ready, then we can scale down old machine sets in a further step.
	//
	// maxScaledDown := allMachinesCount - minAvailable - newMachineSetMachinesUnavailable
	// take into account not only maxUnavailable and any surge machines that have been created, but also unavailable machines from
	// the newMS, so that the unavailable machines from the newMS would not make us scale down old machine sets in a further
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
	// * The new machine set machines crashloop and never become available.
	// * allMachinesCount is 13. minAvailable is 8. newMSMachinesUnavailable is 5.
	// * A node fails and causes one of the oldMS machines to become unavailable. However, 13 - 8 - 5 = 0, so the oldMS won't be scaled down.
	// * The user notices the crashloop and does kubectl rollout undo to rollback.
	// * newMSMachinesUnavailable is 1, since we rolled back to the good machine set, so maxScaledDown = 13 - 8 - 1 = 4. 4 of the crashlooping machines will be scaled down.
	// * The total number of machines will then be 9 and the newMS can be scaled up to 10.
	//
	// case 2:
	// Same example, but pushing a new machine template instead of rolling back (aka "roll over"):
	// * The new machine set created must start with 0 replicas because allMachinesCount is already at 13.
	// * However, newMSMachinesUnavailable would also be 0, so the 2 old machine sets could be scaled down by 5 (13 - 8 - 0), which would then
	// allow the new machine set to be scaled up by 5.
	minAvailable := *(deployment.Spec.Replicas) - maxUnavailable
	newMSUnavailableMachineCount := *(newMS.Spec.Replicas) - newMS.Status.AvailableReplicas
	maxScaledDown := allMachinesCount - minAvailable - newMSUnavailableMachineCount
	if maxScaledDown <= 0 {
		return nil
	}

	// Clean up unhealthy replicas first, otherwise unhealthy replicas will block deployment
	// and cause timeout. See https://github.com/kubernetes/kubernetes/issues/16737
	oldMSs, cleanupCount, err := r.cleanupUnhealthyReplicas(oldMSs, deployment, maxScaledDown)
	if err != nil {
		return nil
	}

	klog.V(4).Infof("Cleaned up unhealthy replicas from old MachineSets by %d", cleanupCount)

	// Scale down old machine sets, need check maxUnavailable to ensure we can scale down
	allMSs = append(oldMSs, newMS)
	scaledDownCount, err := r.scaleDownOldMachineSetsForRollingUpdate(allMSs, oldMSs, deployment)
	if err != nil {
		return err
	}

	klog.V(4).Infof("Scaled down old MachineSets of deployment %s by %d", deployment.Name, scaledDownCount)
	return nil
}

// cleanupUnhealthyReplicas will scale down old machine sets with unhealthy replicas, so that all unhealthy replicas will be deleted.
func (r *ReconcileMachineDeployment) cleanupUnhealthyReplicas(oldMSs []*v1alpha1.MachineSet, deployment *v1alpha1.MachineDeployment, maxCleanupCount int32) ([]*v1alpha1.MachineSet, int32, error) {
	sort.Sort(dutil.MachineSetsByCreationTimestamp(oldMSs))

	// Safely scale down all old machine sets with unhealthy replicas. Replica set will sort the machines in the order
	// such that not-ready < ready, unscheduled < scheduled, and pending < running. This ensures that unhealthy replicas will
	// been deleted first and won't increase unavailability.
	totalScaledDown := int32(0)
	for _, targetMS := range oldMSs {
		if targetMS.Spec.Replicas == nil {
			return nil, 0, errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", targetMS.Name)
		}

		if totalScaledDown >= maxCleanupCount {
			break
		}

		oldMSReplicas := *(targetMS.Spec.Replicas)
		if oldMSReplicas == 0 {
			// cannot scale down this machine set.
			continue
		}

		oldMSAvailableReplicas := targetMS.Status.AvailableReplicas
		klog.V(4).Infof("Found %d available machines in old MS %s/%s", oldMSAvailableReplicas, targetMS.Namespace, targetMS.Name)
		if oldMSReplicas == oldMSAvailableReplicas {
			// no unhealthy replicas found, no scaling required.
			continue
		}

		remainingCleanupCount := maxCleanupCount - totalScaledDown
		unhealthyCount := oldMSReplicas - oldMSAvailableReplicas
		scaledDownCount := integer.Int32Min(remainingCleanupCount, unhealthyCount)
		newReplicasCount := oldMSReplicas - scaledDownCount

		if newReplicasCount > oldMSReplicas {
			return nil, 0, errors.Errorf("when cleaning up unhealthy replicas, got invalid request to scale down %s/%s %d -> %d", targetMS.Namespace, targetMS.Name, oldMSReplicas, newReplicasCount)
		}

		if _, err := r.scaleMachineSet(targetMS, newReplicasCount, deployment); err != nil {
			return nil, totalScaledDown, err
		}

		totalScaledDown += scaledDownCount
	}

	return oldMSs, totalScaledDown, nil
}

// scaleDownOldMachineSetsForRollingUpdate scales down old machine sets when deployment strategy is "RollingUpdate".
// Need check maxUnavailable to ensure availability
func (r *ReconcileMachineDeployment) scaleDownOldMachineSetsForRollingUpdate(allMSs []*v1alpha1.MachineSet, oldMSs []*v1alpha1.MachineSet, deployment *v1alpha1.MachineDeployment) (int32, error) {
	if deployment.Spec.Replicas == nil {
		return 0, errors.Errorf("spec replicas for deployment %v is nil, this is unexpected", deployment.Name)
	}

	maxUnavailable := dutil.MaxUnavailable(*deployment)

	// Check if we can scale down.
	minAvailable := *(deployment.Spec.Replicas) - maxUnavailable

	// Find the number of available machines.
	availableMachineCount := dutil.GetAvailableReplicaCountForMachineSets(allMSs)
	if availableMachineCount <= minAvailable {
		// Cannot scale down.
		return 0, nil
	}

	klog.V(4).Infof("Found %d available machines in deployment %s, scaling down old MSes", availableMachineCount, deployment.Name)

	sort.Sort(dutil.MachineSetsByCreationTimestamp(oldMSs))

	totalScaledDown := int32(0)
	totalScaleDownCount := availableMachineCount - minAvailable
	for _, targetMS := range oldMSs {
		if targetMS.Spec.Replicas == nil {
			return 0, errors.Errorf("spec replicas for machine set %v is nil, this is unexpected", targetMS.Name)
		}

		if totalScaledDown >= totalScaleDownCount {
			// No further scaling required.
			break
		}

		if *(targetMS.Spec.Replicas) == 0 {
			// cannot scale down this MachineSet.
			continue
		}

		// Scale down.
		scaleDownCount := integer.Int32Min(*(targetMS.Spec.Replicas), totalScaleDownCount-totalScaledDown)
		newReplicasCount := *(targetMS.Spec.Replicas) - scaleDownCount
		if newReplicasCount > *(targetMS.Spec.Replicas) {
			return totalScaledDown, errors.Errorf("when scaling down old MS, got invalid request to scale down %s/%s %d -> %d", targetMS.Namespace, targetMS.Name, *(targetMS.Spec.Replicas), newReplicasCount)
		}

		if _, err := r.scaleMachineSet(targetMS, newReplicasCount, deployment); err != nil {
			return totalScaledDown, err
		}

		totalScaledDown += scaleDownCount
	}

	return totalScaledDown, nil
}
