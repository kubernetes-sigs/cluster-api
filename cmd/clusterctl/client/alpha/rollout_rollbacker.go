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
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/controllers/mdutil"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectRollbacker will issue a rollback on the specified cluster-api resource.
func (r *rollout) ObjectRollbacker(proxy cluster.Proxy, tuple util.ResourceTuple, namespace string, toRevision int64) error {
	switch tuple.Resource {
	case MachineDeployment:
		deployment, err := getMachineDeployment(proxy, tuple.Name, namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to get %v/%v", tuple.Resource, tuple.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't rollback a paused MachineDeployment: please run 'clusterctl rollout resume %v/%v' first", tuple.Resource, tuple.Name)
		}
		if err := rollbackMachineDeployment(proxy, deployment, toRevision); err != nil {
			return err
		}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", tuple.Resource, validResourceTypes)
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
	delete(revMSTemplate.Labels, mdutil.DefaultMachineDeploymentUniqueLabelKey)

	d.Spec.Template = revMSTemplate
	return patchHelper.Patch(context.TODO(), d)
}

// findMachineDeploymentRevision finds the specific revision in the machine sets.
func findMachineDeploymentRevision(toRevision int64, allMSs []*clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
	var (
		latestMachineSet   *clusterv1.MachineSet
		latestRevision     = int64(-1)
		previousMachineSet *clusterv1.MachineSet
		previousRevision   = int64(-1)
	)
	for _, ms := range allMSs {
		if v, err := mdutil.Revision(ms); err == nil {
			if toRevision == 0 {
				if latestRevision < v {
					// newest one we've seen so far
					previousRevision = latestRevision
					previousMachineSet = latestMachineSet
					latestRevision = v
					latestMachineSet = ms
				} else if previousRevision < v {
					// second newest one we've seen so far
					previousRevision = v
					previousMachineSet = ms
				}
			} else if toRevision == v {
				return ms, nil
			}
		}
	}

	if toRevision > 0 {
		return nil, errors.Errorf("unable to find specified revision: %v", toRevision)
	}

	if previousMachineSet == nil {
		return nil, errors.Errorf("no rollout history found")
	}
	return previousMachineSet, nil
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func getMachineSetsForDeployment(proxy cluster.Proxy, d *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	log := logf.Log
	c, err := proxy.NewClient()
	if err != nil {
		return nil, err
	}
	// List all MachineSets to find those we own but that no longer match our selector.
	machineSets := &clusterv1.MachineSetList{}
	if err := c.List(context.TODO(), machineSets, client.InNamespace(d.Namespace)); err != nil {
		return nil, err
	}

	filtered := make([]*clusterv1.MachineSet, 0, len(machineSets.Items))
	for idx := range machineSets.Items {
		ms := &machineSets.Items[idx]

		// Skip this MachineSet if its controller ref is not pointing to this MachineDeployment
		if !metav1.IsControlledBy(ms, d) {
			log.V(5).Info("Skipping MachineSet, controller ref does not match MachineDeployment", "machineset", ms.Name)
			continue
		}

		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			log.V(5).Info("Skipping MachineSet, failed to get label selector from spec selector", "machineset", ms.Name)
			continue
		}
		// If a MachineDeployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			log.V(5).Info("Skipping MachineSet as the selector is empty", "machineset", ms.Name)
			continue
		}
		// Skip this MachineSet if selector does not match
		if !selector.Matches(labels.Set(ms.Labels)) {
			log.V(5).Info("Skipping MachineSet, label mismatch", "machineset", ms.Name)
			continue
		}
		filtered = append(filtered, ms)
	}

	return filtered, nil
}
