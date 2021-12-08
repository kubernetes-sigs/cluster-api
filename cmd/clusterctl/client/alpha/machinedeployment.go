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

package alpha

import (
	"strconv"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

// getMachineDeployment retrieves the MachineDeployment object corresponding to the name and namespace specified.
func getMachineDeployment(proxy cluster.Proxy, name, namespace string) (*clusterv1.MachineDeployment, error) {
	mdObj := &clusterv1.MachineDeployment{}
	c, err := proxy.NewClient()
	if err != nil {
		return nil, err
	}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, mdObjKey, mdObj); err != nil {
		return nil, errors.Wrapf(err, "error reading MachineDeployment %s/%s",
			mdObjKey.Namespace, mdObjKey.Name)
	}
	return mdObj, nil
}

// patchMachineDeployemt applies a patch to a machinedeployment.
func patchMachineDeployemt(proxy cluster.Proxy, name, namespace string, patch client.Patch) error {
	cFrom, err := proxy.NewClient()
	if err != nil {
		return err
	}
	mdObj := &clusterv1.MachineDeployment{}
	mdObjKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := cFrom.Get(ctx, mdObjKey, mdObj); err != nil {
		return errors.Wrapf(err, "error reading MachineDeployment %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}

	if err := cFrom.Patch(ctx, mdObj, patch); err != nil {
		return errors.Wrapf(err, "error while patching MachineDeployment %s/%s", mdObj.GetNamespace(), mdObj.GetName())
	}
	return nil
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
		if v, err := revision(ms); err == nil {
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
		return nil, errors.Errorf("unable to find specified MachineDeployment revision: %v", toRevision)
	}

	if previousMachineSet == nil {
		return nil, errors.Errorf("no rollout history found for MachineDeployment")
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
	if err := c.List(ctx, machineSets, client.InNamespace(d.Namespace)); err != nil {
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

func revision(obj runtime.Object) (int64, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[clusterv1.RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
