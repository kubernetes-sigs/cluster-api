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

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/patch"
)

// rolloutOnDelete implements the logic for the OnDelete MachineDeploymentStrategyType.
func (r *Reconciler) rolloutOnDelete(ctx context.Context, md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) error {
	newMS, oldMSs, err := r.getAllMachineSetsAndSyncRevision(ctx, md, msList, true)
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
	if err := r.reconcileNewMachineSetOnDelete(ctx, allMSs, newMS, md); err != nil {
		return err
	}

	if err := r.syncDeploymentStatus(allMSs, newMS, md); err != nil {
		return err
	}

	// Scale down, if we can.
	if err := r.reconcileOldMachineSetsOnDelete(ctx, oldMSs, allMSs, md); err != nil {
		return err
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

// reconcileOldMachineSetsOnDelete handles reconciliation of Old MachineSets associated with the MachineDeployment in the OnDelete MachineDeploymentStrategyType.
func (r *Reconciler) reconcileOldMachineSetsOnDelete(ctx context.Context, oldMSs []*clusterv1.MachineSet, allMSs []*clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) error {
	log := ctrl.LoggerFrom(ctx)
	if deployment.Spec.Replicas == nil {
		return errors.Errorf("spec replicas for MachineDeployment %q/%q is nil, this is unexpected",
			deployment.Namespace, deployment.Name)
	}
	log.V(4).Info("Checking to see if machines have been deleted or are in the process of deleting for old machine sets")
	totalReplicas := mdutil.GetReplicaCountForMachineSets(allMSs)
	scaleDownAmount := totalReplicas - *deployment.Spec.Replicas
	for _, oldMS := range oldMSs {
		log := log.WithValues("MachineSet", klog.KObj(oldMS))
		if oldMS.Spec.Replicas == nil || *oldMS.Spec.Replicas <= 0 {
			log.V(4).Info("fully scaled down")
			continue
		}
		if oldMS.Annotations == nil {
			oldMS.Annotations = map[string]string{}
		}
		if _, ok := oldMS.Annotations[clusterv1.DisableMachineCreateAnnotation]; !ok {
			log.V(4).Info("setting annotation on old MachineSet to disable machine creation")
			patchHelper, err := patch.NewHelper(oldMS, r.Client)
			if err != nil {
				return err
			}
			oldMS.Annotations[clusterv1.DisableMachineCreateAnnotation] = "true"
			if err := patchHelper.Patch(ctx, oldMS); err != nil {
				return err
			}
		}
		selectorMap, err := metav1.LabelSelectorAsMap(&oldMS.Spec.Selector)
		if err != nil {
			log.V(4).Error(err, "failed to convert MachineSet label selector to a map")
			continue
		}
		log.V(4).Info("Fetching Machines associated with MachineSet")
		// Get all Machines linked to this MachineSet.
		allMachinesInOldMS := &clusterv1.MachineList{}
		if err := r.Client.List(ctx,
			allMachinesInOldMS,
			client.InNamespace(oldMS.Namespace),
			client.MatchingLabels(selectorMap),
		); err != nil {
			return errors.Wrap(err, "failed to list machines")
		}
		totalMachineCount := int32(len(allMachinesInOldMS.Items))
		log.V(4).Info("Retrieved machines", "totalMachineCount", totalMachineCount)
		updatedReplicaCount := totalMachineCount - mdutil.GetDeletingMachineCount(allMachinesInOldMS)
		if updatedReplicaCount < 0 {
			return errors.Errorf("negative updated replica count %d for MachineSet %q, this is unexpected", updatedReplicaCount, oldMS.Name)
		}
		machineSetScaleDownAmountDueToMachineDeletion := *oldMS.Spec.Replicas - updatedReplicaCount
		if machineSetScaleDownAmountDueToMachineDeletion < 0 {
			log.V(4).Error(errors.Errorf("unexpected negative scale down amount: %d", machineSetScaleDownAmountDueToMachineDeletion), fmt.Sprintf("Error reconciling MachineSet %s", oldMS.Name))
		}
		scaleDownAmount -= machineSetScaleDownAmountDueToMachineDeletion
		log.V(4).Info("Adjusting replica count for deleted machines", "oldReplicas", oldMS.Spec.Replicas, "newReplicas", updatedReplicaCount)
		log.V(4).Info("Scaling down", "replicas", updatedReplicaCount)
		if err := r.scaleMachineSet(ctx, oldMS, updatedReplicaCount, deployment); err != nil {
			return err
		}
	}
	log.V(4).Info("Finished reconcile of Old MachineSets to account for deleted machines. Now analyzing if there's more potential to scale down")
	for _, oldMS := range oldMSs {
		log := log.WithValues("MachineSet", klog.KObj(oldMS))
		if scaleDownAmount <= 0 {
			break
		}
		if oldMS.Spec.Replicas == nil || *oldMS.Spec.Replicas <= 0 {
			log.V(4).Info("Fully scaled down")
			continue
		}
		updatedReplicaCount := *oldMS.Spec.Replicas
		if updatedReplicaCount >= scaleDownAmount {
			updatedReplicaCount -= scaleDownAmount
			scaleDownAmount = 0
		} else {
			scaleDownAmount -= updatedReplicaCount
			updatedReplicaCount = 0
		}
		log.V(4).Info("Scaling down", "replicas", updatedReplicaCount)
		if err := r.scaleMachineSet(ctx, oldMS, updatedReplicaCount, deployment); err != nil {
			return err
		}
	}
	log.V(4).Info("Finished reconcile of all old MachineSets")
	return nil
}

// reconcileNewMachineSetOnDelete handles reconciliation of the latest MachineSet associated with the MachineDeployment in the OnDelete MachineDeploymentStrategyType.
func (r *Reconciler) reconcileNewMachineSetOnDelete(ctx context.Context, allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet, deployment *clusterv1.MachineDeployment) error {
	// logic same as reconcile logic for RollingUpdate
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
	return r.reconcileNewMachineSet(ctx, allMSs, newMS, deployment)
}
