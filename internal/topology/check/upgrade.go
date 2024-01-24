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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

// IsMachineDeploymentUpgrading determines if the MachineDeployment is upgrading.
// A machine deployment is considered upgrading if at least one of the Machines of this
// MachineDeployment has a different version.
func IsMachineDeploymentUpgrading(ctx context.Context, c client.Reader, md *clusterv1.MachineDeployment) (bool, error) {
	// If the MachineDeployment has no version there is no definitive way to check if it is upgrading. Therefore, return false.
	// Note: This case should not happen.
	if md.Spec.Template.Spec.Version == nil {
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
	mdVersion := *md.Spec.Template.Spec.Version
	// Check if the versions of the all the Machines match the MachineDeployment version.
	for i := range machines.Items {
		machine := machines.Items[i]
		if machine.Spec.Version == nil {
			return false, fmt.Errorf("failed to check if MachineDeployment %s is upgrading: Machine %s has no version", md.Name, machine.Name)
		}
		if *machine.Spec.Version != mdVersion {
			return true, nil
		}
	}
	return false, nil
}

// IsMachinePoolUpgrading determines if the MachinePool is upgrading.
// A machine pool is considered upgrading if at least one of the Machines of this
// MachinePool has a different version.
func IsMachinePoolUpgrading(ctx context.Context, c client.Reader, mp *expv1.MachinePool) (bool, error) {
	// If the MachinePool has no version there is no definitive way to check if it is upgrading. Therefore, return false.
	// Note: This case should not happen.
	if mp.Spec.Template.Spec.Version == nil {
		return false, nil
	}
	mpVersion := *mp.Spec.Template.Spec.Version
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
