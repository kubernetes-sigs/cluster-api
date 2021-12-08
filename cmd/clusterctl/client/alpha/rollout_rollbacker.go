/*
Copyright 2020 The Kubernetes Authors.

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

package alpha

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ObjectRollbacker will issue a rollback on the specified cluster-api resource.
func (r *rollout) ObjectRollbacker(proxy cluster.Proxy, ref corev1.ObjectReference, toRevision int64) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to get %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't rollback a paused MachineDeployment: please run 'clusterctl rollout resume %v/%v' first", ref.Kind, ref.Name)
		}
		if err := rollbackMachineDeployment(proxy, deployment, toRevision); err != nil {
			return err
		}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validResourceTypes)
	}
	return nil
}

// rollbackMachineDeployment will rollback to a previous MachineSet revision used by this MachineDeployment.
func rollbackMachineDeployment(proxy cluster.Proxy, d *clusterv1.MachineDeployment, toRevision int64) error {
	log := logf.Log
	c, err := proxy.NewClient()
	if err != nil {
		return err
	}

	if toRevision < 0 {
		return errors.Errorf("revision number cannot be negative: %v", toRevision)
	}
	msList, err := getMachineSetsForDeployment(proxy, d)
	if err != nil {
		return err
	}
	log.V(7).Info("Found MachineSets", "count", len(msList))
	msForRevision, err := findMachineDeploymentRevision(toRevision, msList)
	if err != nil {
		return err
	}
	log.V(7).Info("Found revision", "revision", msForRevision)
	patchHelper, err := patch.NewHelper(d, c)
	if err != nil {
		return err
	}
	// Copy template into the machinedeployment (excluding the hash)
	revMSTemplate := *msForRevision.Spec.Template.DeepCopy()
	delete(revMSTemplate.Labels, clusterv1.MachineDeploymentUniqueLabel)

	d.Spec.Template = revMSTemplate
	return patchHelper.Patch(ctx, d)
}
