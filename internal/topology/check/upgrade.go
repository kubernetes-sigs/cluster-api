/*
Copyright 2024 The Kubernetes Authors.

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

package check

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// IsMachineDeploymentUpgrading determines if the MachineDeployment is upgrading.
// A machine deployment is considered upgrading if at least one of the Machines of this
// MachineDeployment has a different version.
func IsMachineDeploymentUpgrading(ctx context.Context, c client.Reader, md *clusterv1.MachineDeployment) (bool, error) {
	// If the MachineDeployment has no version there is no definitive way to check if it is upgrading. Therefore, return false.
	// Note: This case should not happen.
	if md.Spec.Template.Spec.Version == "" {
		return false, nil
	}
	selectorMap, err := metav1.LabelSelectorAsMap(&md.Spec.Selector)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if MachineDeployment %s is upgrading: failed to convert label selector to map", md.Name)
	}
	machines := &clusterv1.MachineList{}
	if err := c.List(ctx, machines, client.InNamespace(md.Namespace), client.MatchingLabels(selectorMap)); err != nil {
		return false, errors.Wrapf(err, "failed to check if MachineDeployment %s is upgrading: failed to list Machines", md.Name)
	}
	mdVersion := md.Spec.Template.Spec.Version
	// Check if the versions of the all the Machines match the MachineDeployment version.
	for i := range machines.Items {
		machine := machines.Items[i]
		if machine.Spec.Version == "" {
			return false, fmt.Errorf("failed to check if MachineDeployment %s is upgrading: Machine %s has no version", md.Name, machine.Name)
		}
		if machine.Spec.Version != mdVersion {
			return true, nil
		}
	}
	return false, nil
}

// IsMachineDeploymentRollingOut determines if the MachineDeployment is rolling out based on the given MachineSets.
// A MachineDeployment is considered rolling out if any of the following is true:
//   - its RollingOut condition is true;
//   - the MachineDeployment controller has not observed the latest generation yet;
//   - one of its MachineSets does not match the MachineDeployment machine template and still has replicas,
//     or no MachineSet matches the MachineDeployment machine template.
//
// The MachineSet-based check is required because the RollingOut condition is derived from the Machines'
// UpToDate conditions, which the Machine controller stamps asynchronously: right after the MachineDeployment
// controller observes a template change there is a window where observedGeneration is current but RollingOut
// is still false. Comparing the MachineSets against the MachineDeployment machine template is the same
// synchronous signal the MachineDeployment rollout planner acts on.
func IsMachineDeploymentRollingOut(md *clusterv1.MachineDeployment, machineSets []clusterv1.MachineSet) (bool, error) {
	if conditions.IsTrue(md, clusterv1.MachineDeploymentRollingOutCondition) {
		return true, nil
	}
	if md.Status.ObservedGeneration < md.Generation {
		return true, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(&md.Spec.Selector)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if MachineDeployment %s is rolling out: failed to convert label selector", md.Name)
	}

	upToDateMachineSetExists := false
	for i := range machineSets {
		ms := &machineSets[i]
		if ms.Namespace != md.Namespace || !selector.Matches(labels.Set(ms.Labels)) {
			continue
		}
		if upToDate, _ := mdutil.MachineTemplateUpToDate(&ms.Spec.Template, &md.Spec.Template); upToDate {
			upToDateMachineSetExists = true
			continue
		}
		// A MachineSet not matching the MachineDeployment machine template that still owns or is scheduled to own
		// machines means the rollout replacing those machines is not complete yet.
		if ptr.Deref(ms.Spec.Replicas, 0) > 0 || ptr.Deref(ms.Status.Replicas, 0) > 0 {
			return true, nil
		}
	}
	// Without a MachineSet matching the MachineDeployment machine template the rollout has not been picked up yet;
	// conservatively consider the MachineDeployment rolling out until the MachineDeployment controller creates it.
	return !upToDateMachineSetExists, nil
}

// IsMachinePoolUpgrading determines if the MachinePool is upgrading.
// A machine pool is considered upgrading if at least one of the Machines of this
// MachinePool has a different version.
func IsMachinePoolUpgrading(ctx context.Context, c client.Reader, mp *clusterv1.MachinePool) (bool, error) {
	// If the MachinePool has no version there is no definitive way to check if it is upgrading. Therefore, return false.
	// Note: This case should not happen.
	if mp.Spec.Template.Spec.Version == "" {
		return false, nil
	}
	mpVersion := mp.Spec.Template.Spec.Version
	// Check if the kubelet versions of the MachinePool noderefs match the MachinePool version.
	for _, nodeRef := range mp.Status.NodeRefs {
		node := &corev1.Node{}
		if err := c.Get(ctx, client.ObjectKey{Name: nodeRef.Name}, node); err != nil {
			return false, fmt.Errorf("failed to check if MachinePool %s is upgrading: failed to get Node %s", mp.Name, nodeRef.Name)
		}
		if mpVersion != node.Status.NodeInfo.KubeletVersion {
			return true, nil
		}
	}
	return false, nil
}
