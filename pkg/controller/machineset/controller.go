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
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	machineclientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("MachineSet")

// +controller:group=cluster,version=v1alpha1,kind=MachineSet,resource=machinesets
type MachineSetControllerImpl struct {
	builders.DefaultControllerFns

	// machineClient a client that knows how to consume Machine resources
	machineClient machineclientset.Interface

	// machineSetsLister indexes properties about MachineSet
	machineSetsLister listers.MachineSetLister

	// machineLister holds a lister that knows how to list Machines from a cache
	machineLister listers.MachineLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineSetControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	c.machineSetsLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineSets().Lister()
	c.machineLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()

	var err error
	c.machineClient, err = machineclientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error building clientset for machineClient: %v", err)
	}
}

// Reconcile holds the controller's business logic.
// it makes sure that the current state is equal to the desired state.
// note that the current state of the cluster is calculated based on the number of machines
// that are owned by the given machineSet (key).
func (c *MachineSetControllerImpl) Reconcile(machineSet *v1alpha1.MachineSet) error {
	filteredMachines, err := c.getMachines(machineSet)
	if err != nil {
		return err
	}

	return c.syncReplicas(machineSet, filteredMachines)
}

func (c *MachineSetControllerImpl) Get(namespace, name string) (*v1alpha1.MachineSet, error) {
	return c.machineSetsLister.MachineSets(namespace).Get(name)
}

// syncReplicas essentially scales machine resources up and down.
func (c *MachineSetControllerImpl) syncReplicas(machineSet *v1alpha1.MachineSet, machines []*v1alpha1.Machine) error {
	// Take ownership of machines if not already owned.
	for _, machine := range machines {
		if shouldAdopt(machineSet, machine) {
			c.adoptOrphan(machineSet, machine)
		}
	}

	var result error
	currentMachineCount := int32(len(machines))
	desiredReplicas := *machineSet.Spec.Replicas
	diff := int(currentMachineCount - desiredReplicas)

	if diff < 0 {
		diff *= -1
		for i := 0; i < diff; i++ {
			glog.V(2).Infof("creating a machine ( spec.replicas(%d) > currentMachineCount(%d) )", desiredReplicas, currentMachineCount)
			machine, err := c.createMachine(machineSet)
			if err != nil {
				return err
			}
			_, err = c.machineClient.ClusterV1alpha1().Machines(machineSet.Namespace).Create(machine)
			if err != nil {
				glog.Errorf("unable to create a machine = %s, due to %v", machine.Name, err)
				result = err
			}
		}
	} else if diff > 0 {
		for i := 0; i < diff; i++ {
			glog.V(2).Infof("deleting a machine ( spec.replicas(%d) < currentMachineCount(%d) )", desiredReplicas, currentMachineCount)
			// TODO: Define machines deletion policies.
			// see: https://github.com/kubernetes/kube-deploy/issues/625
			machineToDelete := machines[i]
			err := c.machineClient.ClusterV1alpha1().Machines(machineSet.Namespace).Delete(machineToDelete.Name, &metav1.DeleteOptions{})
			if err != nil {
				glog.Errorf("unable to delete a machine = %s, due to %v", machineToDelete.Name, err)
				result = err
			}
		}
	}

	return result
}

// createMachine creates a machine resource.
// the name of the newly created resource is going to be created by the API server, we set the generateName field
func (c *MachineSetControllerImpl) createMachine(machineSet *v1alpha1.MachineSet) (*v1alpha1.Machine, error) {
	gv := v1alpha1.SchemeGroupVersion
	machine := &v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		},
		ObjectMeta: machineSet.Spec.Template.ObjectMeta,
		Spec:       machineSet.Spec.Template.Spec,
	}
	machine.ObjectMeta.GenerateName = fmt.Sprintf("%s-", machineSet.Name)
	machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, controllerKind),}

	return machine, nil
}

// getMachines returns a list of machines that match on machineSet.Spec.Selector
func (c *MachineSetControllerImpl) getMachines(machineSet *v1alpha1.MachineSet) ([]*v1alpha1.Machine, error) {
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		return nil, err
	}
	filteredMachines, err := c.machineLister.List(selector)
	if err != nil {
		return nil, err
	}
	return filteredMachines, err
}

func shouldAdopt(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	// Do nothing if the machine is being deleted.
	if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
		glog.V(2).Infof("Skipping machine (%v), as it is being deleted.", machine.Name)
		return false
	}

	// Machine owned by another controller.
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		glog.Warningf("Skipping machine (%v), as it is owned by someone else.", machine.Name)
		return false
	}

	// Machine we control.
	if metav1.IsControlledBy(machine, machineSet) {
		return false
	}

	return true
}

func (c *MachineSetControllerImpl) adoptOrphan(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) {
	// Add controller reference.
	ownerRefs := machine.ObjectMeta.GetOwnerReferences()
	if ownerRefs == nil {
		ownerRefs = []metav1.OwnerReference{}
	}

	newRef := *metav1.NewControllerRef(machineSet, controllerKind)
	ownerRefs = append(ownerRefs, newRef)
	machine.ObjectMeta.SetOwnerReferences(ownerRefs)
	if _, err := c.machineClient.ClusterV1alpha1().Machines(machineSet.Namespace).Update(machine); err != nil {
		glog.Warningf("Failed to update machine owner reference. %v", err)
	}
}
