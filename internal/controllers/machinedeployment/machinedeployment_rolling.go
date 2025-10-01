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

	if err := planner.Plan(ctx); err != nil {
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

// Plan determine how to proceed with the rollout if we are not yet at the desired state.
func (p *rolloutPlanner) Plan(ctx context.Context) error {
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
	if err := p.reconcileOldMachineSets(ctx); err != nil {
		return err
	}

	// This funcs tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
	// Note. this func must be called after computing scale up/down intent for all the MachineSets.
	// Note. this func only address deadlock due to unavailable machines not getting deleted on oldMSs, e.g. due to a wrong configuration.
	// unblocking deadlock when unavailable machines exists only on oldMSs, is required also because failures on old machines set are not remediated by MHC.
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

func (p *rolloutPlanner) reconcileOldMachineSets(ctx context.Context) error {
	allMSs := append(p.oldMSs, p.newMS)

	// no op if there are no replicas on old machinesets
	if mdutil.GetReplicaCountForMachineSets(p.oldMSs) == 0 {
		return nil
	}

	maxUnavailable := mdutil.MaxUnavailable(*p.md)
	minAvailable := max(ptr.Deref(p.md.Spec.Replicas, 0)-maxUnavailable, 0)

	totReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)

	// Find the number of available machines.
	totAvailableReplicas := ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(allMSs), 0)

	// Find the number of pending scale down from previous reconcile/from current reconcile;
	// This is required because whenever we are reducing the number of replicas, this operation could further impact availability e.g.
	// - in case of regular rollout, there is no certainty about which machine is going to be deleted (and if this machine is currently available or not):
	// 	 - e.g. MS controller is going to delete first machines with deletion annotation; also MS controller has a slight different notion of unavailable as of now.
	// - in case of in-place rollout, in-place upgrade are always assumed as impacting availability (they can always fail).
	totPendingScaleDown := int32(0)
	for _, ms := range allMSs {
		scaleIntent := ptr.Deref(ms.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[ms.Name]; ok {
			scaleIntent = min(scaleIntent, v)
		}

		// NOTE: we are counting only pending scale down from the current status.replicas (so scale down of actual machines).
		if scaleIntent < ptr.Deref(ms.Status.Replicas, 0) {
			totPendingScaleDown += max(ptr.Deref(ms.Status.Replicas, 0)-scaleIntent, 0)
		}
	}

	// Compute the total number of replicas that can be scaled down.
	// Exit immediately if there is no room for scaling down.
	totalScaleDownCount := max(totReplicas-totPendingScaleDown-minAvailable, 0)
	if totalScaleDownCount <= 0 {
		return nil
	}

	sort.Sort(mdutil.MachineSetsByCreationTimestamp(p.oldMSs))

	// Scale down only unavailable replicas / up to residual totalScaleDownCount.
	// NOTE: we are scaling up unavailable machines first in order to increase chances for the rollout to progress;
	// however, the MS controller might have different opinion on which machines to scale down.
	// As a consequence, the scale down operation must continuously assess if reducing the number of replicas
	// for an older MS could further impact availability under the assumption than any scale down could further impact availability (same as above).
	// if reducing the number of replicase might lead to breaching minAvailable, scale down extent must be limited accordingly.
	totalScaleDownCount, totAvailableReplicas = p.scaleDownOldMSs(ctx, totalScaleDownCount, totAvailableReplicas, minAvailable, false)

	// Then scale down old MS up to zero replicas / up to residual totalScaleDownCount.
	// NOTE: also in this case, continuously assess if reducing the number of replicase could further impact availability,
	// and if necessary, limit scale down extent to ensure the operation respects minAvailable limits.
	_, _ = p.scaleDownOldMSs(ctx, totalScaleDownCount, totAvailableReplicas, minAvailable, true)

	return nil
}

func (p *rolloutPlanner) scaleDownOldMSs(ctx context.Context, totalScaleDownCount, totAvailableReplicas, minAvailable int32, scaleToZero bool) (int32, int32) {
	log := ctrl.LoggerFrom(ctx)

	for _, oldMS := range p.oldMSs {
		// No op if there is no scaling down left.
		if totalScaleDownCount <= 0 {
			break
		}

		// No op if this MS has been already scaled down to zero.
		scaleIntent := ptr.Deref(oldMS.Spec.Replicas, 0)
		if v, ok := p.scaleIntents[oldMS.Name]; ok {
			scaleIntent = min(scaleIntent, v)
		}

		if scaleIntent <= 0 {
			continue
		}

		// Compute the scale down extent by considering either unavailable replicas or, if scaleToZero is set, all replicas.
		// In both cases, scale down is limited to totalScaleDownCount.
		maxScaleDown := max(scaleIntent-ptr.Deref(oldMS.Status.AvailableReplicas, 0), 0)
		if scaleToZero {
			maxScaleDown = scaleIntent
		}
		scaleDown := min(maxScaleDown, totalScaleDownCount)

		// Exit if there is no room for scaling down the MS.
		if scaleDown == 0 {
			continue
		}

		// Before scaling down validate if the operation will lead to a breach to minAvailability
		// In order to do so, consider how many machines will be actually deleted, and consider this operation as impacting availability;
		// if the projected state breaches minAvailability, reduce the scale down extend accordingly.
		availableMachineScaleDown := int32(0)
		if ptr.Deref(oldMS.Status.AvailableReplicas, 0) > 0 {
			newScaleIntent := scaleIntent - scaleDown
			machineScaleDownIntent := max(ptr.Deref(oldMS.Status.Replicas, 0)-newScaleIntent, 0)
			if totAvailableReplicas-machineScaleDownIntent < minAvailable {
				availableMachineScaleDown = max(totAvailableReplicas-minAvailable, 0)
				scaleDown = scaleDown - machineScaleDownIntent + availableMachineScaleDown

				totAvailableReplicas = max(totAvailableReplicas-availableMachineScaleDown, 0)
			}
		}

		if scaleDown > 0 {
			newScaleIntent := max(scaleIntent-scaleDown, 0)
			log.V(5).Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas (-%d)", oldMS.Name, newScaleIntent, scaleDown), "machineset", client.ObjectKeyFromObject(oldMS).String())
			p.scaleIntents[oldMS.Name] = newScaleIntent
			totalScaleDownCount = max(totalScaleDownCount-scaleDown, 0)
		}
	}

	return totalScaleDownCount, totAvailableReplicas
}

// This funcs tries to detect and address the case when a rollout is not making progress because both scaling down and scaling up are blocked.
// Note. this func must be called after computing scale up/down intent for all the MachineSets.
// Note. this func only address deadlock due to unavailable machines not getting deleted on oldMSs, e.g. due to a wrong configuration.
// unblocking deadlock when unavailable machines exists only on oldMSs, is required also because failures on old machines set are not remediated by MHC.
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

	// if there are scale operation in progress, no deadlock.
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
	// Note: a rollout cannot be unblocked if new machines do not become available.
	// Note: if the replicas on the newMS are not going to become available for any issue either:
	// - automatic remediation can help in addressing temporary failures.
	// - user intervention is required to fix more permanent issues e.g. to fix a wrong configuration.
	if ptr.Deref(p.newMS.Status.AvailableReplicas, 0) != ptr.Deref(p.newMS.Status.Replicas, 0) {
		return
	}

	// At this point we can assume there is a deadlock that can be remediated by breaching maxUnavailability constraint
	// and scaling down an oldMS with unavailable machines by one.
	//
	// Note: in most cases this is only a formal violation of maxUnavailability, because there is a good changes
	// that the machine that will be deleted is one of the unavailable machines
	for _, oldMS := range p.oldMSs {
		if ptr.Deref(oldMS.Status.AvailableReplicas, 0) == ptr.Deref(oldMS.Status.Replicas, 0) || ptr.Deref(oldMS.Spec.Replicas, 0) == 0 {
			continue
		}

		newScaleIntent := max(ptr.Deref(oldMS.Spec.Replicas, 0)-1, 0)
		log.Info(fmt.Sprintf("Setting scale down intent for %s to %d replicas (-%d) to unblock rollout stuck due to unavailable machine on oldMS only", oldMS.Name, newScaleIntent, 1), "machineset", client.ObjectKeyFromObject(oldMS).String())
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
