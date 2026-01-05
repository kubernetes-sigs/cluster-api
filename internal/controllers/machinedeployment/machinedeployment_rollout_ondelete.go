/*
Copyright 2021 The Kubernetes Authors.

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

	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/collections"
)

// rolloutOnDelete reconcile machine sets controlled by a MachineDeployment that is using the OnDelete strategy.
func (r *Reconciler) rolloutOnDelete(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet, machines collections.Machines, templateExists bool) error {
	planner := newRolloutPlanner(r.Client, r.RuntimeClient, r.canUpdateMachineSetCache)
	if err := planner.init(ctx, md, msList, machines.UnsortedList(), true, templateExists); err != nil {
		return err
	}

	if err := planner.planOnDelete(ctx); err != nil {
		return err
	}

	if err := r.createOrUpdateMachineSetsAndSyncMachineDeploymentRevision(ctx, planner); err != nil {
		return err
	}

	newMS := planner.newMS
	oldMSs := planner.oldMSs
	allMSs := append(oldMSs, newMS)

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

// planOnDelete determine how to proceed with the rollout when using the OnDelete strategy if we are not yet at the desired state.
func (p *rolloutPlanner) planOnDelete(ctx context.Context) error {
	// Scale up, if we can.
	if err := p.reconcileNewMachineSet(ctx); err != nil {
		return err
	}

	// Scale down, if we can.
	p.reconcileOldMachineSetsOnDelete(ctx)
	return nil
}

// reconcileOldMachineSetsOnDelete handles reconciliation of Old MachineSets associated with the MachineDeployment in the OnDelete rollout strategy.
func (p *rolloutPlanner) reconcileOldMachineSetsOnDelete(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)
	oldMachinesCount := mdutil.GetReplicaCountForMachineSets(p.oldMSs)
	if oldMachinesCount == 0 {
		// Can't scale down further
		return
	}

	// Determine if there are more Machines than MD.spec.replicas, e.g. due to a scale down in MD.
	newMSReplicas := ptr.Deref(p.newMS.Spec.Replicas, 0)
	if v, ok := p.scaleIntents[p.newMS.Name]; ok {
		newMSReplicas = v
	}
	totReplicas := oldMachinesCount + newMSReplicas
	totalScaleDownCount := max(totReplicas-ptr.Deref(p.md.Spec.Replicas, 0), 0)

	// Sort oldMSs so the system will start deleting from the oldest MS first.
	sort.Sort(mdutil.MachineSetsByCreationTimestamp(p.oldMSs))

	// Start scaling down old machine sets to acknowledge spec.replicas without corresponding status.replicas.
	// Note: spec.replicas without corresponding status.replicas exists
	// - after a user manually deletes a replica
	// - when a newMS not yet fully provisioned suddenly becomes an oldMS.
	// In both cases spec.replicas without corresponding status.replicas should be dropped, no matter
	// if there are replicas to be scaled down due to a scale down in MD or not.
	// However, just in case there are replicas to be scaled down due to a scale down in MD, deleted replicas should
	// be deducted from the totalScaleDownCount.
	for _, oldMS := range p.oldMSs {
		// No op if this MS has been already scaled down to zero.
		if ptr.Deref(oldMS.Spec.Replicas, 0) <= 0 {
			continue
		}

		scaleDownCount := max(ptr.Deref(oldMS.Spec.Replicas, 0)-ptr.Deref(oldMS.Status.Replicas, 0), 0)
		if scaleDownCount > 0 {
			newScaleIntent := max(ptr.Deref(oldMS.Spec.Replicas, 0)-scaleDownCount, 0)
			p.addNotef(oldMS, "scale down to align to existing Machines")
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", oldMS.Name, newScaleIntent, scaleDownCount), "MachineSet", klog.KObj(oldMS))
			p.scaleIntents[oldMS.Name] = newScaleIntent

			totalScaleDownCount -= scaleDownCount
		}
	}

	// Scale down additional replicas if replicas removed in the for loop above were not enough to align to MD replicas.
	for _, oldMS := range p.oldMSs {
		// No op if there is no scaling down left.
		if totalScaleDownCount <= 0 {
			break
		}

		// No op if this MS has been already scaled down to zero.
		scaleIntent := ptr.Deref(oldMS.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[oldMS.Name]; ok {
			scaleIntent = v
		}

		if scaleIntent <= 0 {
			continue
		}

		scaleDownCount := min(scaleIntent, totalScaleDownCount)
		if scaleDownCount > 0 {
			newScaleIntent := max(ptr.Deref(oldMS.Spec.Replicas, 0)-scaleDownCount, 0)
			p.addNotef(oldMS, "scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas")
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for MachineSet %s to %d replicas (-%d)", oldMS.Name, newScaleIntent, scaleDownCount), "MachineSet", klog.KObj(oldMS))
			p.scaleIntents[oldMS.Name] = newScaleIntent

			totalScaleDownCount -= scaleDownCount
		}
	}
}
