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
	"fmt"
	"reflect"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("MachineDeployment")

// +controller:group=cluster,version=v1alpha1,kind=MachineDeployment,resource=machinedeployments
type MachineDeploymentControllerImpl struct {
	builders.DefaultControllerFns

	// machineClient a client that knows how to consume Machine resources
	machineClient clientset.Interface

	// lister indexes properties about MachineDeployment
	mLister  listers.MachineLister
	mdLister listers.MachineDeploymentLister
	msLister listers.MachineSetLister

	informers *sharedinformers.SharedInformers
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineDeploymentControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machinedeployments labels
	c.mLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()
	c.msLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineSets().Lister()
	c.mdLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineDeployments().Lister()

	arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineSets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addMachineSet,
		UpdateFunc: c.updateMachineSet,
		DeleteFunc: c.deleteMachineSet,
	})

	mc, err := clientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error building clientset for machineClient: %v", err)
	}
	c.machineClient = mc

	c.informers = arguments.GetSharedInformers()

	c.waitForCacheSync()
}

func (c *MachineDeploymentControllerImpl) waitForCacheSync() {
	glog.Infof("Waiting for caches to sync for machine deployment controller")
	stopCh := make(chan struct{})
	mListerSynced := c.informers.Factory.Cluster().V1alpha1().Machines().Informer().HasSynced
	msListerSynced := c.informers.Factory.Cluster().V1alpha1().MachineSets().Informer().HasSynced
	mdListerSynced := c.informers.Factory.Cluster().V1alpha1().MachineDeployments().Informer().HasSynced
	if !cache.WaitForCacheSync(stopCh, mListerSynced, msListerSynced, mdListerSynced) {
		glog.Warningf("Unable to sync caches for machine deployment controller")
		return
	}
	glog.Infof("Caches are synced for machine deployment controller")
}

func (c *MachineDeploymentControllerImpl) getMachineSetsForDeployment(d *v1alpha1.MachineDeployment) ([]*v1alpha1.MachineSet, error) {
	// List all MachineSets to find those we own but that no longer match our
	// selector.
	msList, err := c.msLister.MachineSets(d.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	// TODO: flush out machine set adoption.

	var filteredMS []*v1alpha1.MachineSet
	for _, ms := range msList {
		if metav1.GetControllerOf(ms) == nil || (metav1.GetControllerOf(ms) != nil && !metav1.IsControlledBy(ms, d)) {
			glog.V(4).Infof("%s not controlled by %v", ms.Name, d.Name)
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			glog.Errorf("Skipping machineset %v, failed to get label selector from spec selector.", ms.Name)
			continue
		}
		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() {
			glog.Warningf("Skipping machineset %v as the selector is empty.", ms.Name)
			continue
		}
		if !selector.Matches(labels.Set(ms.Labels)) {
			glog.V(4).Infof("Skipping machineset %v, label mismatch.", ms.Name)
			continue
		}
		filteredMS = append(filteredMS, ms)
	}
	return filteredMS, nil
}

// Reconcile handles reconciling of machine deployment
func (c *MachineDeploymentControllerImpl) Reconcile(u *v1alpha1.MachineDeployment) error {
	// Deep-copy otherwise we are mutating our cache.
	d := u.DeepCopy()

	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(d.Spec.Selector, &everything) {
		if d.Status.ObservedGeneration < d.Generation {
			d.Status.ObservedGeneration = d.Generation
			if _, err := c.machineClient.ClusterV1alpha1().MachineDeployments(d.Namespace).UpdateStatus(d); err != nil {
				glog.Warningf("Failed to update status for deployment %v. %v", d.Name, err)
				return err
			}
		}
		return nil
	}

	msList, err := c.getMachineSetsForDeployment(d)
	if err != nil {
		return err
	}

	machineMap, err := c.getMachineMapForDeployment(d, msList)
	if err != nil {
		return err
	}

	if d.DeletionTimestamp != nil {
		return c.sync(d, msList, machineMap)
	}

	if d.Spec.Paused {
		return c.sync(d, msList, machineMap)
	}

	switch d.Spec.Strategy.Type {
	case common.RollingUpdateMachineDeploymentStrategyType:
		return c.rolloutRolling(d, msList, machineMap)
	}
	return fmt.Errorf("unexpected deployment strategy type: %s", d.Spec.Strategy.Type)
}

func (c *MachineDeploymentControllerImpl) Get(namespace, name string) (*v1alpha1.MachineDeployment, error) {
	return c.mdLister.MachineDeployments(namespace).Get(name)
}

// addMachineSet enqueues the deployment that manages a MachineSet when the MachineSet is created.
func (c *MachineDeploymentControllerImpl) addMachineSet(obj interface{}) {
	ms := obj.(*v1alpha1.MachineSet)

	if ms.DeletionTimestamp != nil {
		// On a restart of the controller manager, it's possible for an object to
		// show up in a state that is already pending deletion.
		c.deleteMachineSet(ms)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(ms); controllerRef != nil {
		d := c.resolveControllerRef(ms.Namespace, controllerRef)
		if d == nil {
			return
		}
		glog.V(4).Infof("MachineSet %s added for deployment %v.", ms.Name, d.Name)
		c.enqueue(d)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching Deployments and sync
	// them to see if anyone wants to adopt it.
	mds := c.getMachineDeploymentsForMachineSet(ms)
	if len(mds) == 0 {
		return
	}
	glog.V(4).Infof("Orphan MachineSet %s added.", ms.Name)
	for _, d := range mds {
		c.enqueue(d)
	}
}

// getMachineDeploymentsForMachineSet returns a list of Deployments that potentially
// match a MachineSet.
func (c *MachineDeploymentControllerImpl) getMachineDeploymentsForMachineSet(ms *v1alpha1.MachineSet) []*v1alpha1.MachineDeployment {
	if len(ms.Labels) == 0 {
		glog.Warningf("no machine deployments found for MachineSet %v because it has no labels", ms.Name)
		return nil
	}

	dList, err := c.mdLister.MachineDeployments(ms.Namespace).List(labels.Everything())
	if err != nil {
		glog.Warningf("failed to list machine deployments, %v", err)
		return nil
	}

	var deployments []*v1alpha1.MachineDeployment
	for _, d := range dList {
		selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
		if err != nil {
			continue
		}
		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(ms.Labels)) {
			continue
		}
		deployments = append(deployments, d)
	}

	return deployments
}

// updateMachineSet figures out what deployment(s) manage a MachineSet when the MachineSet
// is updated and wake them up. If anything on the MachineSet has changed we need to
// reconcile it's current MachineDeployment. If the MachineSet's controller reference has
// changed, we must also reconcile it's old MachineDeployment.
func (c *MachineDeploymentControllerImpl) updateMachineSet(old, cur interface{}) {
	curMS := cur.(*v1alpha1.MachineSet)
	oldMS := old.(*v1alpha1.MachineSet)
	if curMS.ResourceVersion == oldMS.ResourceVersion {
		// Periodic resync will send update events for all known machine sets.
		// Two different versions of the same machine set will always have different RVs.
		return
	}

	curControllerRef := metav1.GetControllerOf(curMS)
	oldControllerRef := metav1.GetControllerOf(oldMS)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if d := c.resolveControllerRef(oldMS.Namespace, oldControllerRef); d != nil {
			c.enqueue(d)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		d := c.resolveControllerRef(curMS.Namespace, curControllerRef)
		if d == nil {
			return
		}
		glog.V(4).Infof("MachineSet %s updated.", curMS.Name)
		c.enqueue(d)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	labelChanged := !reflect.DeepEqual(curMS.Labels, oldMS.Labels)
	if labelChanged || controllerRefChanged {
		mds := c.getMachineDeploymentsForMachineSet(curMS)
		if len(mds) == 0 {
			return
		}
		glog.V(4).Infof("Orphan MachineSet %s updated.", curMS.Name)
		for _, d := range mds {
			c.enqueue(d)
		}
	}
}

// deleteMachineSet enqueues the deployment that manages a MachineSet when
// the MachineSet is deleted.
func (c *MachineDeploymentControllerImpl) deleteMachineSet(obj interface{}) {
	ms := obj.(*v1alpha1.MachineSet)

	controllerRef := metav1.GetControllerOf(ms)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	d := c.resolveControllerRef(ms.Namespace, controllerRef)
	if d == nil {
		return
	}
	glog.V(4).Infof("MachineSet %s deleted.", ms.Name)
	c.enqueue(d)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (c *MachineDeploymentControllerImpl) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *v1alpha1.MachineDeployment {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		glog.Warningf("Failed to get machine deployment, controller ref had unexpected kind %v, expected %v", controllerRef.Kind, controllerKind.Kind)
		return nil
	}
	d, err := c.mdLister.MachineDeployments(namespace).Get(controllerRef.Name)
	if err != nil {
		glog.Warningf("Failed to get machine deployment with name %v", controllerRef.Name)
		return nil
	}
	if d.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		glog.Warningf("Failed to get machine deployment, UID mismatch. controller ref UID: %v, found machine deployment UID: %v", controllerRef.UID, d.UID)
		return nil
	}
	return d
}

// getMachineMapForDeployment returns the Machines managed by a Deployment.
//
// It returns a map from MachineSet UID to a list of Machines controlled by that MS,
// according to the Machine's ControllerRef.
func (c *MachineDeploymentControllerImpl) getMachineMapForDeployment(d *v1alpha1.MachineDeployment, msList []*v1alpha1.MachineSet) (map[types.UID]*v1alpha1.MachineList, error) {
	// Get all Machines that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsSelector(&d.Spec.Selector)
	if err != nil {
		return nil, err
	}
	machines, err := c.mLister.Machines(d.Namespace).List(selector)
	if err != nil {
		return nil, err
	}
	// Group Machines by their controller (if it's in msList).
	machineMap := make(map[types.UID]*v1alpha1.MachineList, len(msList))
	for _, ms := range msList {
		machineMap[ms.UID] = &v1alpha1.MachineList{}
	}
	for _, machine := range machines {
		// Do not ignore inactive Machines because Recreate Deployments need to verify that no
		// Machines from older versions are running before spinning up new Machines.
		controllerRef := metav1.GetControllerOf(machine)
		if controllerRef == nil {
			continue
		}
		// Only append if we care about this UID.
		if machineList, ok := machineMap[controllerRef.UID]; ok {
			machineList.Items = append(machineList.Items, *machine)
		}
	}
	return machineMap, nil
}

func (c *MachineDeploymentControllerImpl) enqueue(d *v1alpha1.MachineDeployment) {
	key, err := cache.MetaNamespaceKeyFunc(d)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %#v: %v", d, err))
		return
	}

	c.informers.WorkerQueues["MachineDeployment"].Queue.Add(key)
}
