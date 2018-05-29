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

package machineset

import (
	"fmt"
	"strings"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func (c *MachineSetControllerImpl) reconcileMachine(key string) error {
	glog.V(4).Infof("Reconcile machine from machineset: %v", key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		glog.Errorf("Failed to split key: %v. %v", key, err)
		return err
	}

	m, err := c.clusterAPIClient.ClusterV1alpha1().Machines(namespace).Get(name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		glog.Errorf("Unable to retrieve Machine %v from store: %v", key, err)
		return err
	}

	// If it has a ControllerRef, enqueue the controller ref machine set
	if controllerRef := metav1.GetControllerOf(m); controllerRef != nil {
		ms := c.resolveControllerRef(namespace, controllerRef)
		if ms == nil {
			glog.V(4).Infof("Found no machineset from controller ref for machine %v", m.Name)
			return nil
		}
		if err := c.enqueue(ms); err != nil {
			return fmt.Errorf("failed to enqueue machine set %v due to change to machine %v. %v", ms.Name, m.Name, err)
		}
		return nil
	}

	mss := c.getMachineSetsForMachine(m)
	if len(mss) == 0 {
		glog.V(4).Infof("Found no machine set for machine: %v", m.Name)
		return nil
	}
	var errstrings []string
	for _, ms := range mss {
		if err := c.enqueue(ms); err != nil {
			errstrings = append(errstrings, err.Error())
		}
	}
	if len(errstrings) > 0 {
		return fmt.Errorf("failed to enqueue machine sets due to change to machine %v. %v", m.Name, strings.Join(errstrings, "; "))
	}
	return nil
}

func (c *MachineSetControllerImpl) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *v1alpha1.MachineSet {
	if controllerRef.Kind != controllerKind.Kind {
		glog.Warningf("Found unexpected controller ref kind, got %v, expected %v", controllerRef.Kind, controllerKind.Kind)
		return nil
	}
	ms, err := c.machineSetLister.MachineSets(namespace).Get(controllerRef.Name)
	if err != nil {
		glog.Warningf("Failed to get machine set with name %v.", controllerRef.Name)
		return nil
	}
	if ms.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the ControllerRef points to.
		glog.Warningf("Found unexpected UID, got %v, expected %v.", ms.UID, controllerRef.UID)
		return nil
	}
	return ms
}

func (c *MachineSetControllerImpl) getMachineSetsForMachine(m *v1alpha1.Machine) []*v1alpha1.MachineSet {
	if len(m.Labels) == 0 {
		glog.Warningf("No machine sets found for Machine %v because it has no labels", m.Name)
		return nil
	}

	msList, err := c.machineSetLister.MachineSets(m.Namespace).List(labels.Everything())
	if err != nil {
		glog.Errorf("Failed to list machine sets, %v", err)
		return nil
	}

	var mss []*v1alpha1.MachineSet
	for _, ms := range msList {
		if hasMatchingLabels(ms, m) {
			mss = append(mss, ms)
		}
	}

	return mss
}
